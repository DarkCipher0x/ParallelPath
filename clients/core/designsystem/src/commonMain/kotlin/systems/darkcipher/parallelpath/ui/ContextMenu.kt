package systems.darkcipher.parallelpath.ui

import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.ui.composed
import androidx.compose.ui.input.pointer.PointerEventType
import androidx.compose.ui.input.pointer.changedToDownIgnoreConsumed
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.pointer.PointerKeyboardModifiers
import androidx.compose.ui.input.pointer.PointerType
import androidx.compose.ui.input.pointer.isCtrlPressed
import androidx.compose.ui.input.pointer.isMetaPressed
import androidx.compose.ui.input.pointer.isSecondaryPressed
import androidx.compose.ui.input.pointer.isShiftPressed
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.DpOffset
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp

/** One entry in a context menu (right-click on a computer, a long press on a phone). */
sealed interface MenuEntry

/** An action. [shortcut] is only shown (the screen handles the keys); [hint] says why it's off when [enabled] is false. */
data class MenuItem(
    val label: String,
    val icon: ImageVector? = null,
    val shortcut: String? = null,
    val enabled: Boolean = true,
    val danger: Boolean = false,
    val hint: String? = null,
    val onClick: () -> Unit,
) : MenuEntry

/** A submenu (opens in place, with a way back - it works the same with a finger as with a mouse). */
data class MenuGroup(val label: String, val icon: ImageVector? = null, val items: List<MenuEntry>) : MenuEntry

data object MenuDivider : MenuEntry

/** Builds a menu, leaving out nulls (actions that don't apply) and doubled or trailing dividers. */
fun menu(vararg entries: MenuEntry?): List<MenuEntry> {
    val out = mutableListOf<MenuEntry>()
    for (e in entries) {
        if (e == null || (e == MenuDivider && (out.isEmpty() || out.last() == MenuDivider))) continue
        out += e
    }
    while (out.lastOrNull() == MenuDivider) out.removeAt(out.lastIndex)
    return out
}

/** Which modifier keys were held for a click (Ctrl/Cmd adds to a selection, Shift selects a range). */
data class ClickKeys(val ctrl: Boolean = false, val shift: Boolean = false)

private fun PointerKeyboardModifiers.keys() = ClickKeys(ctrl = isCtrlPressed || isMetaPressed, shift = isShiftPressed)

/**
 * Something with a context menu: right-click (or a long press on a touch screen) opens [items] where it was
 * pressed. [onClick] gets the modifier keys held; [onDoubleClick] is a double click or double tap; with
 * [onDragStart] a mouse drag begins a drag (the explorer's drag-to-move).
 */
@Composable
fun ContextMenuArea(
    items: () -> List<MenuEntry>,
    modifier: Modifier = Modifier,
    // For a background (the empty space around a list's rows): only clicks and right-clicks no row took.
    yieldToChildren: Boolean = false,
    onClick: ((ClickKeys) -> Unit)? = null,
    onDoubleClick: (() -> Unit)? = null,
    onContext: (() -> Unit)? = null,
    onDragStart: ((Offset) -> Unit)? = null,
    onDrag: ((Offset) -> Unit)? = null,
    onDragEnd: (() -> Unit)? = null,
    content: @Composable () -> Unit,
) {
    var at by remember { mutableStateOf<Offset?>(null) }
    var size by remember { mutableStateOf(IntSize.Zero) }
    var lastTap by remember { mutableStateOf(0L) }
    // The newest handlers, without restarting a gesture that's under way when the screen recomposes.
    val click by rememberUpdatedState(onClick)
    val double by rememberUpdatedState(onDoubleClick)
    val context by rememberUpdatedState(onContext)
    val dragStart by rememberUpdatedState(onDragStart)
    val drag by rememberUpdatedState(onDrag)
    val dragEnd by rememberUpdatedState(onDragEnd)
    Box(modifier.onSizeChanged { size = it }.pointerInput(yieldToChildren) {
        awaitEachGesture {
            // Any button going down (awaitFirstDown only reports the primary mouse button, not a right-click).
            var event: androidx.compose.ui.input.pointer.PointerEvent
            do {
                event = awaitPointerEvent()
            } while (event.changes.none { it.changedToDownIgnoreConsumed() })
            val down = event.changes.first { it.changedToDownIgnoreConsumed() }
            if (yieldToChildren && down.isConsumed) return@awaitEachGesture
            if (event.buttons.isSecondaryPressed) {
                down.consume()
                context?.invoke()
                at = down.position
                waitForUpOrCancellation()
                return@awaitEachGesture
            }
            if (down.type == PointerType.Touch) {
                // A long press opens the menu; a tap is a click; a move is left to the list (it scrolls).
                var up: androidx.compose.ui.input.pointer.PointerInputChange? = null
                val slop = viewConfiguration.touchSlop
                val finished = withTimeoutOrNull(viewConfiguration.longPressTimeoutMillis) {
                    while (true) {
                        val e = awaitPointerEvent()
                        val c = e.changes.firstOrNull { it.id == down.id }
                        if (c == null || c.isConsumed || (c.position - down.position).getDistance() > slop) break // a scroll, not a press
                        if (!c.pressed) { up = c; break }
                    }
                    true
                }
                if (finished == null) {
                    if (!yieldToChildren && currentEvent.changes.any { it.pressed && !it.isConsumed }) {
                        context?.invoke()
                        at = down.position
                        currentEvent.changes.forEach { it.consume() }
                        waitForUpOrCancellation()
                    }
                    return@awaitEachGesture
                }
                val u = up ?: return@awaitEachGesture // scrolled away
                if (yieldToChildren && u.isConsumed) return@awaitEachGesture
                u.consume()
                tap(u.uptimeMillis, lastTap, ClickKeys(), click, double).also { lastTap = it }
                return@awaitEachGesture
            }
            val keys = event.keyboardModifiers.keys()
            // A mouse: a click, or (moved far enough while held) a drag.
            var dragging = false
            while (true) {
                val e = awaitPointerEvent()
                val c = e.changes.firstOrNull { it.id == down.id } ?: break
                if (!c.pressed) {
                    if (dragging) {
                        c.consume()
                        dragEnd?.invoke()
                    } else if (!(yieldToChildren && c.isConsumed)) {
                        c.consume()
                        tap(c.uptimeMillis, lastTap, keys, click, double).also { lastTap = it }
                    }
                    break
                }
                if (dragStart != null && !dragging && (c.position - down.position).getDistance() > viewConfiguration.touchSlop) {
                    dragging = true
                    dragStart?.invoke(down.position)
                }
                if (dragging) {
                    c.consume()
                    drag?.invoke(c.position)
                }
            }
        }
    }) {
        content()
        val open = at
        if (open != null) {
            val d = LocalDensity.current
            ContextMenu(items(), DpOffset(with(d) { open.x.toDp() }, with(d) { (open.y - size.height).toDp() })) { at = null }
        }
    }
}

/** A right-click only (other presses pass through untouched): for things that already have a tap and long-press. */
fun Modifier.onRightClick(action: (Offset) -> Unit): Modifier = composed {
    val a by rememberUpdatedState(action)
    pointerInput(Unit) {
        awaitEachGesture {
            val e = awaitPointerEvent()
            val c = e.changes.firstOrNull { it.changedToDownIgnoreConsumed() } ?: return@awaitEachGesture
            if (e.type == PointerEventType.Press && e.buttons.isSecondaryPressed) {
                c.consume()
                a(c.position)
            }
        }
    }
}

private fun tap(now: Long, last: Long, keys: ClickKeys, onClick: ((ClickKeys) -> Unit)?, onDouble: (() -> Unit)?): Long {
    if (onDouble != null && last > 0 && now - last < 400 && !keys.ctrl && !keys.shift) {
        onDouble()
        return 0L
    }
    onClick?.invoke(keys)
    return now
}

/** The menu itself, anchored to its parent at [offset] (for a ⋮ button: DpOffset.Zero). */
@Composable
fun ContextMenu(items: List<MenuEntry>, offset: DpOffset, onDismiss: () -> Unit) {
    var group by remember(items) { mutableStateOf<MenuGroup?>(null) }
    DropdownMenu(expanded = true, onDismissRequest = onDismiss, offset = offset, containerColor = pp.panel,
        modifier = Modifier.widthIn(min = 220.dp)) {
        val g = group
        if (g != null) {
            DropdownMenuItem(text = { Text(g.label, color = pp.fgDim, style = MaterialTheme.typography.labelLarge) },
                leadingIcon = { Icon(Icons.AutoMirrored.Outlined.ArrowBack, "Back", tint = pp.fgDim) }, onClick = { group = null })
            HorizontalDivider(color = pp.line)
        }
        for (e in g?.items ?: items) {
            when (e) {
                MenuDivider -> HorizontalDivider(color = pp.line)
                is MenuGroup -> DropdownMenuItem(
                    text = { Text(e.label, color = pp.fg) },
                    leadingIcon = e.icon?.let { { Icon(it, null, tint = pp.fgDim, modifier = Modifier.size(20.dp)) } },
                    trailingIcon = { Icon(Icons.AutoMirrored.Outlined.KeyboardArrowRight, null, tint = pp.fgMute) },
                    onClick = { group = e },
                )
                is MenuItem -> DropdownMenuItem(
                    text = {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(e.label, color = when {
                                    !e.enabled -> pp.fgMute
                                    e.danger -> pp.danger
                                    else -> pp.fg
                                })
                            }
                            if (e.shortcut != null) {
                                Spacer(Modifier.width(28.dp))
                                Text(e.shortcut, color = pp.fgMute, style = MaterialTheme.typography.labelSmall)
                            }
                        }
                    },
                    leadingIcon = e.icon?.let { { Icon(it, null, tint = if (e.danger && e.enabled) pp.danger else pp.fgDim, modifier = Modifier.size(20.dp)) } },
                    enabled = e.enabled,
                    onClick = { onDismiss(); e.onClick() },
                )
            }
        }
        // Why some actions are greyed out, said once.
        val why = (g?.items ?: items).filterIsInstance<MenuItem>().filter { !it.enabled }.mapNotNull { it.hint }.distinct()
        if (why.isNotEmpty()) {
            HorizontalDivider(color = pp.line)
            why.forEach { Text(it, color = pp.fgMute, style = MaterialTheme.typography.labelSmall,
                modifier = Modifier.widthIn(max = 280.dp).padding(horizontal = 16.dp, vertical = 8.dp)) }
        }
    }
}
