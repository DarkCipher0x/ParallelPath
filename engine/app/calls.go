package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"pp/core/envelope"
	"pp/core/pqc"
	"pp/core/wire"
	"pp/engine/remote"
)

// Calls. The media itself is WebRTC between the devices (directly, or through the server's TURN relay), but every
// audio and video frame is additionally encrypted end to end (AES-256-GCM, in the call page's frame transform)
// with keys from a random per-call key. That key, and every call-setup message, travel only inside messages
// sealed with a key derived from the family's epoch key - which is shared by post-quantum ML-KEM-1024 + P-384 key
// encapsulation - so a recording of the call can't be decrypted later even by a quantum computer, and the server
// (which relays everything) never sees a key, a stream or what was said. The family's record gets a "call" item
// saying who called, when, and for how long; the call itself is never stored anywhere.

// callState is a call this device is part of (memory only, never stored).
type callState struct {
	Space    string
	ID       string
	Key      []byte
	Video    bool
	Caller   string // person
	Outgoing bool
	Started  time.Time
}

// callEvent is an incoming call-setup message, as given to the app and the call page.
type callEvent struct {
	Space      string          `json:"space"`
	Call       string          `json:"call"`
	Type       string          `json:"type"`
	FromPerson string          `json:"from_person"`
	FromName   string          `json:"from_name"`
	FromDevice string          `json:"from_device"`
	Video      bool            `json:"video,omitempty"`
	SDP        string          `json:"sdp,omitempty"`
	Candidate  json.RawMessage `json:"candidate,omitempty"`
	Peers      []string        `json:"peers,omitempty"`
	FamilyName string          `json:"family_name,omitempty"`
}

// signalBody is the plaintext of a call-setup message.
type signalBody struct {
	Type      string          `json:"type"` // ring | accept | decline | busy | offer | answer | ice | join | hangup
	Call      string          `json:"call"`
	Video     bool            `json:"video,omitempty"`
	Key       []byte          `json:"key,omitempty"` // ring: the call's media key
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	Peers     []string        `json:"peers,omitempty"` // join: the devices already in the call
	Sent      time.Time       `json:"sent"`
}

type sealedSignal struct {
	Epoch uint64 `json:"epoch"`
	Box   []byte `json:"box"`
}

var signalTypes = map[string]bool{"ring": true, "accept": true, "decline": true, "busy": true, "offer": true, "answer": true,
	"ice": true, "join": true, "hangup": true}

func signalAAD(space, from, to string) []byte {
	return []byte("pp/signal/v1|" + space + "|" + from + "|" + to)
}

func signalKey(epochKey []byte) []byte { return pqc.Derive(epochKey, nil, "call-signal", pqc.KeySize) }

// sendSignal seals a call-setup message with the family's current epoch key and has the server relay it.
func (e *Engine) sendSignal(space, toDevice string, b signalBody) error {
	if !signalTypes[b.Type] || wire.CheckID(b.Call) != nil {
		return errors.New("invalid call message")
	}
	b.Sent = time.Now().UTC()
	sv, err := e.spaceView(space)
	if err != nil {
		return err
	}
	e.mu.Lock()
	v, dev := e.v, e.dev
	e.mu.Unlock()
	if v == nil || dev == nil {
		return errLocked
	}
	ek, ok := e.epochKey(v, space, sv.Space.Epoch)
	if !ok {
		return errors.New("you can't call yet: a parent still needs to approve this device")
	}
	k := signalKey(ek)
	pqc.Wipe(ek)
	defer pqc.Wipe(k)
	plain, _ := json.Marshal(b)
	box, err := pqc.Seal(k, signalAAD(space, dev.DeviceID(), toDevice), plain)
	pqc.Wipe(plain)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(sealedSignal{Epoch: sv.Space.Epoch, Box: box})
	return e.call("POST", "/api/v1/spaces/"+space+"/signal", map[string]any{"to_device": toDevice, "payload": payload}, nil)
}

// onSignal opens a relayed call-setup message and passes it to the app and any open call page.
func (e *Engine) onSignal(s *remote.Signal) {
	e.mu.Lock()
	v, dev := e.v, e.dev
	e.mu.Unlock()
	if v == nil || dev == nil || s.FromDevice == dev.DeviceID() || (s.ToDevice != "" && s.ToDevice != dev.DeviceID()) {
		return
	}
	var ss sealedSignal
	if json.Unmarshal(s.Payload, &ss) != nil {
		return
	}
	ek, ok := e.epochKey(v, s.SpaceID, ss.Epoch)
	if !ok {
		return
	}
	k := signalKey(ek)
	pqc.Wipe(ek)
	plain, err := pqc.Open(k, signalAAD(s.SpaceID, s.FromDevice, s.ToDevice), ss.Box)
	pqc.Wipe(k)
	if err != nil {
		return // not from someone holding the family's keys
	}
	var b signalBody
	err = json.Unmarshal(plain, &b)
	pqc.Wipe(plain)
	if err != nil || !signalTypes[b.Type] || wire.CheckID(b.Call) != nil || time.Since(b.Sent) > 2*time.Minute {
		return
	}
	all, _ := e.loadDecoded(s.SpaceID)
	names := e.namesFor(s.SpaceID, all)
	ev := callEvent{Space: s.SpaceID, Call: b.Call, Type: b.Type, FromPerson: s.FromPerson, FromName: names.of(s.FromPerson),
		FromDevice: s.FromDevice, Video: b.Video, SDP: b.SDP, Candidate: b.Candidate, Peers: b.Peers, FamilyName: names.space}
	e.mu.Lock()
	if b.Type == "ring" {
		if s.FromPerson == e.person { // another of my own devices is calling: not for me to answer
			e.mu.Unlock()
			return
		}
		if len(b.Key) != pqc.KeySize {
			e.mu.Unlock()
			return
		}
		e.mu.Unlock()
		if !e.mayJoinCall(s.SpaceID) {
			return // a whole-family call this person isn't allowed to join (they're called in a conversation instead)
		}
		e.mu.Lock()
		if _, known := e.calls[b.Call]; !known {
			e.calls[b.Call] = &callState{Space: s.SpaceID, ID: b.Call, Key: b.Key, Video: b.Video, Caller: s.FromPerson, Started: time.Now()}
		}
	}
	subs := make([]chan callEvent, 0, len(e.callSub))
	for ch := range e.callSub {
		subs = append(subs, ch)
	}
	e.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
	e.listen("call", ev)
}

func (e *Engine) subscribeCalls() chan callEvent {
	ch := make(chan callEvent, 256)
	e.mu.Lock()
	e.callSub[ch] = struct{}{}
	e.mu.Unlock()
	return ch
}

func (e *Engine) unsubscribeCalls(ch chan callEvent) {
	e.mu.Lock()
	delete(e.callSub, ch)
	e.mu.Unlock()
}

// CallData is the content of a "call" item in the family record.
type CallData struct {
	Call    string `json:"call"`
	Video   bool   `json:"video,omitempty"`
	Event   string `json:"event"` // start | end
	Seconds int    `json:"seconds,omitempty"`
	Missed  bool   `json:"missed,omitempty"`
}

// mayJoinCall reports whether this person may take part in a call in a space: always in a private conversation (the
// server checks the family's permissions); in the family itself, if they may ring the whole family.
func (e *Engine) mayJoinCall(space string) bool {
	sv, err := e.spaceView(space)
	if err != nil {
		return false
	}
	if sv.Space.Parent != "" {
		return true
	}
	p := e.myPerms(sv)
	return sv.Me.Role == "parent" || p.FamilyCall || p.Call == "anyone"
}

// CallStart starts a call to the rest of the family: a fresh media key, a "ring" to every device, and a record.
func (e *Engine) CallStart(space string, video bool) (map[string]string, error) {
	sv, err := e.spaceView(space)
	if err != nil {
		return nil, err
	}
	if sv.Space.Parent == "" && !e.mayJoinCall(space) {
		return nil, errors.New("you can call people one by one (Call… › choose who), not the whole family")
	}
	if sv.Space.Parent == "" && sv.Me.ReadOnly && sv.Me.Role != "child" {
		return nil, errors.New("your access to this family is read-only")
	}
	id := wire.NewID()
	key := pqc.NewKey()
	e.mu.Lock()
	e.calls[id] = &callState{Space: space, ID: id, Key: key, Video: video, Caller: e.person, Outgoing: true, Started: time.Now()}
	e.mu.Unlock()
	if err := e.sendSignal(space, "", signalBody{Type: "ring", Call: id, Video: video, Key: key}); err != nil {
		e.mu.Lock()
		delete(e.calls, id)
		e.mu.Unlock()
		return nil, err
	}
	// The record (who, when). An older server that doesn't know "call" items yet doesn't stop the call.
	_, _ = e.post(space, envelope.Draft{Kind: envelope.KindCall, Body: envelope.Body{Data: jsonData(CallData{Call: id, Video: video, Event: "start"})}})
	return map[string]string{"call": id}, nil
}

// CallSignalInput is a call-setup message from the call page.
type CallSignalInput struct {
	Space     string          `json:"space"`
	Call      string          `json:"call"`
	ToDevice  string          `json:"to_device,omitempty"`
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	Peers     []string        `json:"peers,omitempty"`
}

// CallSignal sends a call-setup message (accept, decline, offer, answer, ICE candidate, join, hangup).
func (e *Engine) CallSignal(in CallSignalInput) error {
	if in.Type == "ring" {
		return errors.New("use call_start")
	}
	if len(in.SDP) > 32<<10 || len(in.Candidate) > 4<<10 || len(in.Peers) > 16 {
		return errors.New("call message too large")
	}
	return e.sendSignal(in.Space, in.ToDevice, signalBody{Type: in.Type, Call: in.Call, SDP: in.SDP, Candidate: in.Candidate, Peers: in.Peers})
}

// CallSession is what the call page needs: the media key (never shown to anyone), and who this device is.
func (e *Engine) CallSession(space, call string) (map[string]any, error) {
	e.mu.Lock()
	c := e.calls[call]
	dev, person := e.dev, e.person
	e.mu.Unlock()
	if c == nil || c.Space != space || dev == nil {
		return nil, errors.New("that call has ended")
	}
	all, _ := e.loadDecoded(space)
	names := e.namesFor(space, all)
	return map[string]any{
		"key":      base64.StdEncoding.EncodeToString(c.Key),
		"video":    c.Video,
		"device":   dev.DeviceID(),
		"person":   person,
		"name":     names.of(person),
		"outgoing": c.Outgoing,
		"caller":   names.of(c.Caller),
		"family":   names.space,
	}, nil
}

// CallEnd hangs up: tells the others, records how long it lasted (or that it was missed), and forgets the key.
func (e *Engine) CallEnd(space, call string, seconds int, missed bool) error {
	e.mu.Lock()
	c := e.calls[call]
	delete(e.calls, call)
	e.mu.Unlock()
	if c != nil {
		pqc.Wipe(c.Key)
	}
	_ = e.sendSignal(space, "", signalBody{Type: "hangup", Call: call})
	e.listen("call", callEvent{Space: space, Call: call, Type: "ended"}) // for the app: this device is free again
	if c != nil && c.Outgoing {
		_, _ = e.post(space, envelope.Draft{Kind: envelope.KindCall,
			Body: envelope.Body{Data: jsonData(CallData{Call: call, Video: c.Video, Event: "end", Seconds: max(seconds, 0), Missed: missed})}})
	}
	return nil
}

// CallDecline turns down an incoming call (the caller's page is told).
func (e *Engine) CallDecline(space, call string) error {
	e.mu.Lock()
	if c := e.calls[call]; c != nil {
		pqc.Wipe(c.Key)
	}
	delete(e.calls, call)
	e.mu.Unlock()
	return e.sendSignal(space, "", signalBody{Type: "decline", Call: call})
}

// CallTurn gets short-lived credentials for the server's call relay.
func (e *Engine) CallTurn() (json.RawMessage, error) {
	var res json.RawMessage
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	e.mu.Lock()
	srv := e.srv
	e.mu.Unlock()
	if srv == nil {
		return nil, errLocked
	}
	if err := srv.Call(ctx, "GET", "/api/v1/turn", nil, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// callMethods are the engine methods the call page may use (the local call bridge allows nothing else).
var callMethods = map[string]bool{"call_session": true, "call_signal": true, "call_end": true, "call_turn": true, "call_decline": true}
