// Package milenage implements the 3GPP Milenage algorithm set (TS 35.205/206)
// over AES-128, from scratch. OPc is what the provisioner needs; f1–f5 are
// included so we can validate against the 3GPP known-answer vectors.
package milenage

import (
	"crypto/aes"
	"fmt"
)

// rotate constants (bytes) and c-constants per TS 35.206.
var (
	r = [5]uint{64, 0, 32, 64, 96}
	c = [5][16]byte{
		{},         // c1 = 0
		{15: 0x01}, // c2
		{15: 0x02}, // c3
		{15: 0x04}, // c4
		{15: 0x08}, // c5
	}
)

func encryptBlock(k, in []byte) ([]byte, error) {
	if len(k) != 16 || len(in) != 16 {
		return nil, fmt.Errorf("milenage: block/key must be 16 bytes (k=%d in=%d)", len(k), len(in))
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 16)
	block.Encrypt(out, in)
	return out, nil
}

func xor16(a, b []byte) []byte {
	out := make([]byte, 16)
	for i := 0; i < 16; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

// rotBytes rotates a 16-byte block left by n bits (n a multiple of 8 for Milenage).
func rotBytes(in []byte, bits uint) []byte {
	n := (bits / 8) % 16
	out := make([]byte, 16)
	for i := 0; i < 16; i++ {
		out[i] = in[(uint(i)+n)%16]
	}
	return out
}

// ComputeOPc returns OPc = OP XOR AES-E[K](OP).
func ComputeOPc(k, op []byte) ([]byte, error) {
	if len(k) != 16 || len(op) != 16 {
		return nil, fmt.Errorf("milenage: k and op must be 16 bytes")
	}
	enc, err := encryptBlock(k, op)
	if err != nil {
		return nil, err
	}
	return xor16(op, enc), nil
}

// temp = E[K](RAND XOR OPc)
func temp(k, opc, rand []byte) ([]byte, error) {
	return encryptBlock(k, xor16(rand, opc))
}

// F1 returns MAC-A (f1) and MAC-S (f1*).
func F1(k, opc, rand, sqn, amf []byte) (macA, macS []byte, err error) {
	if len(sqn) != 6 || len(amf) != 2 {
		return nil, nil, fmt.Errorf("milenage: sqn must be 6 bytes, amf 2 bytes")
	}
	tmp, err := temp(k, opc, rand)
	if err != nil {
		return nil, nil, err
	}
	// in1 = SQN||AMF||SQN||AMF (16 bytes)
	in1 := make([]byte, 16)
	copy(in1[0:6], sqn)
	copy(in1[6:8], amf)
	copy(in1[8:14], sqn)
	copy(in1[14:16], amf)

	tmp2 := xor16(in1, opc)
	tmp2 = rotBytes(tmp2, r[0])
	tmp2 = xor16(tmp2, c[0][:])
	out1, err := encryptBlock(k, xor16(tmp, tmp2))
	if err != nil {
		return nil, nil, err
	}
	out1 = xor16(out1, opc)
	return out1[0:8], out1[8:16], nil
}

// F2345 returns RES (f2, 8B), CK (f3, 16B), IK (f4, 16B), AK (f5, 6B).
func F2345(k, opc, rand []byte) (res, ck, ik, ak []byte, err error) {
	tmp, err := temp(k, opc, rand)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// f2 & f5
	out2, err := encryptBlock(k, xor16(rotBytes(xor16(tmp, opc), r[1]), c[1][:]))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	out2 = xor16(out2, opc)
	// f3
	out3, err := encryptBlock(k, xor16(rotBytes(xor16(tmp, opc), r[2]), c[2][:]))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	out3 = xor16(out3, opc)
	// f4
	out4, err := encryptBlock(k, xor16(rotBytes(xor16(tmp, opc), r[3]), c[3][:]))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	out4 = xor16(out4, opc)
	return out2[8:16], out3, out4, out2[0:6], nil
}
