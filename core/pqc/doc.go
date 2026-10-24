// Package pqc holds every cryptographic primitive ParallelPath uses. Nothing else in the code base calls a
// cipher, KEM or signature directly, so the whole suite can be reviewed (and swapped) in one place.
//
// All public-key operations are hybrid: post-quantum (ML-KEM-1024 / ML-DSA-87, NIST category 5) combined with a
// classical algorithm (P-384 / Ed25519), so a break of either one alone doesn't break ParallelPath.
// Everything comes from Go's standard library (crypto/hpke, crypto/mldsa, crypto/ed25519, crypto/sha3), except
// Argon2id, which comes from golang.org/x/crypto.
package pqc
