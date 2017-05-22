package ed25519

import (
	"bytes"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"math/big"
	"testing"

	"github.com/gtank/ed25519/internal/edwards25519"
)

// TEST MATH

// d is a constant from the curve equation. it's easy to find the value in multiple formats.
var d_fe = edwards25519.FieldElement{
	-10913610, 13857413, -15372611, 6949391, 114729, -8787816, -6275908, -3247719, -18696448, -12055116,
}
var d_bn, _ = new(big.Int).SetString("37095705934669439343138083508754565189542113879843219016388785533085940283555", 10)

// 2*d
var d2_fe = edwards25519.FieldElement{
	-21827239, -5839606, -30745221, 13898782, 229458, 15978800, -12551817, -6495438, 29715968, 9444199,
}
var d2_bn = new(big.Int).Mul(big.NewInt(2), d_bn)

// A is a constant from the Montgomery form of curve25519.
var A_fe = edwards25519.FieldElement{
	486662, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
var A_bn, _ = new(big.Int).SetString("486662", 10)

var feConvTable = []struct {
	name string
	fe   edwards25519.FieldElement
	bn   *big.Int
}{
	{
		name: "d",
		fe:   d_fe,
		bn:   d_bn,
	},
	{
		name: "d2",
		fe:   d2_fe,
		bn:   d2_bn,
	},
	{
		name: "A",
		fe:   A_fe,
		bn:   A_bn,
	},
}

func TestFeFromBig(t *testing.T) {
	var _ = Ed25519().(ed25519Curve)
	var fe edwards25519.FieldElement
	for _, c := range feConvTable {
		feFromBig(&fe, c.bn)
		for i := 0; i < len(fe); i++ {
			if fe[i] != c.fe[i] {
				t.Error("bn->fe radix conversion failed")
			}
		}
	}
}

func TestFeToBig(t *testing.T) {
	var curve = Ed25519().(ed25519Curve)
	var bn, expectedBN = new(big.Int), new(big.Int)
	for _, c := range feConvTable {
		expectedBN.Mod(c.bn, curve.P)
		feToBig(bn, &c.fe)
		if bn.Cmp(expectedBN) != 0 {
			t.Errorf("fe->bn radix conversion failed: %s", c.name)
		}
	}
}

func TestRadixRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	var curve = Ed25519().(ed25519Curve)
	var fe edwards25519.FieldElement
	var out = new(big.Int)

	for i := 0; i < 1000; i++ {
		n, err := rand.Int(rand.Reader, curve.P)
		if err != nil {
			t.Fatal(err)
		}
		feFromBig(&fe, n)
		feToBig(out, &fe)
		if n.Cmp(out) != 0 {
			t.Errorf("fe<>bn failed for %x", n.Bytes())
		}
	}
}

func TestIsOnCurve(t *testing.T) {
	ed := Ed25519()
	if !ed.IsOnCurve(ed.Params().Gx, ed.Params().Gy) {
		t.Error("ed25519 base point not on curve")
	}

	x, y := new(big.Int), new(big.Int)
	if ed.IsOnCurve(x, y) {
		t.Error("(0,0) is not on the curve")
	}
}

func TestAdd(t *testing.T) {
	c := Ed25519().(ed25519Curve)
	Bx, By := c.Params().Gx, c.Params().Gy
	B2x, B2y := c.Add(Bx, By, Bx, By)

	if !c.IsOnCurve(B2x, B2y) {
		t.Error("B+B is not on the curve")
	}
}

func TestDouble(t *testing.T) {
	c := Ed25519().(ed25519Curve)
	Gx, Gy := c.Params().Gx, c.Params().Gy
	G2x, G2y := c.Double(Gx, Gy)

	Ax, Ay := c.Add(Gx, Gy, Gx, Gy)

	if Ax.Cmp(G2x) != 0 || Ay.Cmp(G2y) != 0 {
		t.Errorf("double(B) != B+B")
	}
}

// Test vector generated by instrumenting x/crypto/ed25519 GenerateKey
// seed: c240344fcc6615dda52da98149377ad2b13fdba2bc39a50ba9f3afb2cbd4abaa
// expanded: f04154b9d80963bb4c76214ece8a1049bdd16fbfc5003aff9835a59643ace276
// public: 65a8343a83ec15e55050f12fc22f2c81a4fe7327c8da1524441f9ce5e5bc27dd

func TestScalarMultBase(t *testing.T) {
	c := Ed25519()

	// endian-swapped because x/crypto assumes this is always little endian from raw bytes
	// and scalarFromBytes assumes it's coming from big.Int Bytes()
	a, _ := hex.DecodeString("76e2ac4396a53598ff3a00c5bf6fd1bd49108ace4e21764cbb6309d8b95441f0")
	if len(a) != 32 {
		t.Errorf("failed decoding")
	}

	Ax, Ay := c.ScalarBaseMult(a)

	if !c.IsOnCurve(Ax, Ay) {
		t.Error("scalarmultbase result was off-curve")
	}

	var A edwards25519.ExtendedGroupElement
	pub, _ := hex.DecodeString("65a8343a83ec15e55050f12fc22f2c81a4fe7327c8da1524441f9ce5e5bc27dd")
	var pubBytes [32]byte
	copy(pubBytes[:], pub)
	A.FromBytes(&pubBytes)
	Bx, By := extendedToAffine(&A)

	if Ax.Cmp(Bx) != 0 || Ay.Cmp(By) != 0 {
		t.Error("scalarmultbase disagrees with x/crypto/ed25519")
	}
}

func TestScalarMultBaseIdentity(t *testing.T) {
	var c = Ed25519()
	var one = new(big.Int).Set(bigOne)
	Ax, Ay := c.ScalarBaseMult(one.Bytes())

	if Ax.Cmp(c.Params().Gx) != 0 || Ay.Cmp(c.Params().Gy) != 0 {
		t.Errorf("precomputed 1*B != B")
	}

	Ax, Ay = c.ScalarMult(Ax, Ay, one.Bytes())

	if Ax.Cmp(c.Params().Gx) != 0 || Ay.Cmp(c.Params().Gy) != 0 {
		t.Errorf("arbitrary 1*B != B")
	}
}

func TestScalarMultBaseInfinity(t *testing.T) {
	c := Ed25519()
	a, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	if len(a) != 32 {
		t.Errorf("failed decoding")
	}

	Ax, Ay := c.ScalarBaseMult(a)

	if !c.IsOnCurve(Ax, Ay) {
		t.Error("scalarmultbase result was off-curve")
	}

	if Ax.Cmp(bigZero) != 0 || Ay.Cmp(bigOne) != 0 {
		t.Error("scalarmultbase by 0 was not point at infinity")
	}
}

func TestScalarMultsAgreeAtInfinity(t *testing.T) {
	c := Ed25519()
	a, _ := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	if len(a) != 32 {
		t.Errorf("failed decoding")
	}

	Ax, Ay := c.ScalarBaseMult(a)
	Bx, By := c.ScalarMult(c.Params().Gx, c.Params().Gy, a)

	if Ax.Cmp(Bx) != 0 || Ay.Cmp(By) != 0 {
		t.Error("scalarmultbase disagrees with scalarmult")
	}
}

func TestScalarMultsAgreeElsewhere(t *testing.T) {
	c := Ed25519()
	// head -c 32 /dev/urandom | sha256sum
	a, _ := hex.DecodeString("c07eea55b3322f15099b6cf4d2b7e99d3d0fa6807f6fc7a46b5f7cb78daad4e0")

	Ax, Ay := c.ScalarBaseMult(a)
	Bx, By := c.ScalarMult(c.Params().Gx, c.Params().Gy, a)

	if Ax.Cmp(Bx) != 0 || Ay.Cmp(By) != 0 {
		t.Error("scalarmultbase disagrees with scalarmult")
	}

	if !c.IsOnCurve(Bx, By) {
		t.Error("scalarmult is returning off-curve points")
	}
}

// TEST INTERFACE

func TestMarshalingRoundTrip(t *testing.T) {
	ed := Ed25519()

	a, _ := hex.DecodeString("c07eea55b3322f15099b6cf4d2b7e99d3d0fa6807f6fc7a46b5f7cb78daad4e0")
	Ax, Ay := ed.ScalarBaseMult(a)

	if !ed.IsOnCurve(Ax, Ay) {
		t.Error("scalarBaseMult is returning off-curve points")
	}

	sec1A := elliptic.Marshal(ed, Ax, Ay)
	Bx, By := elliptic.Unmarshal(ed, sec1A)

	if Ax.Cmp(Bx) != 0 || Ay.Cmp(By) != 0 {
		t.Error("point did not survive elliptic.Marshal roundtrip")
	}

	if !testing.Short() {
		for i := 0; i < 100; i++ {
			_, err := io.ReadFull(rand.Reader, a)
			if err != nil {
				t.Fatal(err)
			}
			Ax, Ay := ed.ScalarBaseMult(a)

			if !ed.IsOnCurve(Ax, Ay) {
				t.Error("scalarBaseMult is returning off-curve points")
			}

			sec1A := elliptic.Marshal(ed, Ax, Ay)
			Bx, By := elliptic.Unmarshal(ed, sec1A)

			if Ax.Cmp(Bx) != 0 || Ay.Cmp(By) != 0 {
				t.Error("point did not survive elliptic.Marshal roundtrip")
			}
		}
	}
}

// TEST APPLICATION

func generateKey(r io.Reader) (sk *[32]byte, pk []byte, err error) {
	if r == nil {
		r = rand.Reader
	}

	ed := Ed25519()

	sk = new([32]byte)
	_, err = io.ReadFull(r, sk[:])
	if err != nil {
		return nil, nil, err
	}

	digest := sha512.Sum512(sk[:])
	digest[0] &= 248
	digest[31] &= 127
	digest[31] |= 64

	// take the first half of expanded bytes & reverse; big.Int expects big-endian
	reverseDigest := reverse32(digest[:32])
	scalar := new(big.Int).SetBytes(reverseDigest)

	// A = a*B
	Ax, Ay := ed.ScalarBaseMult(scalar.Bytes())

	// This works with the standard elliptic package marshaling
	publicKey := elliptic.Marshal(ed, Ax, Ay)

	// but we can also render the compressed Edwards format
	compressedEdwardsY := make([]byte, 32)

	// x, y here will be big-endian byte strings
	x, y := publicKey[1:33], publicKey[33:]

	// RFC 8032: To form the encoding of the point, copy the least significant
	// bit of the x-coordinate to the most significant bit of the final octet.
	copy(compressedEdwardsY[:], y)
	compressedEdwardsY[0] |= x[31] << 7
	compressedEdwardsY = reverse32(compressedEdwardsY)

	return sk, compressedEdwardsY, err
}

func reverse32(b []byte) []byte {
	var tmp = make([]byte, 32)
	for i := 0; i <= 31; i++ {
		tmp[i] = b[31-i]
	}
	return tmp
}

// Test vector generated by instrumenting x/crypto/ed25519 GenerateKey(). These
// are raw values. The edwards code interprets them as little-endian, so they
// need to be reversed before use with big.Int.
var genKeyTest = struct {
	seed, expanded, public string
}{
	seed:     "c240344fcc6615dda52da98149377ad2b13fdba2bc39a50ba9f3afb2cbd4abaa",
	expanded: "f04154b9d80963bb4c76214ece8a1049bdd16fbfc5003aff9835a59643ace276",
	public:   "65a8343a83ec15e55050f12fc22f2c81a4fe7327c8da1524441f9ce5e5bc27dd",
}

func TestEdDSAGenerateKey(t *testing.T) {
	fakeRandom, _ := hex.DecodeString(genKeyTest.seed)
	fakeReader := bytes.NewBuffer(fakeRandom)

	sk, pk, err := generateKey(fakeReader)
	if err != nil {
		t.Fatal(err)
	}

	expectedPK, _ := hex.DecodeString(genKeyTest.public)
	if !bytes.Equal(sk[:], fakeRandom) || !bytes.Equal(pk, expectedPK) {
		t.Error("generateKey output did not match test vector")
	}
}

// BENCHMARKS

func BenchmarkIsOnCurve(b *testing.B) {
	ed := Ed25519()
	Gx, Gy := ed.Params().Gx, ed.Params().Gy
	for i := 0; i < b.N; i++ {
		if !ed.IsOnCurve(Gx, Gy) {
			b.Error("ed25519 base point not on curve")
		}
	}
}

func BenchmarkAdd(b *testing.B) {
	ed := Ed25519()
	Gx, Gy := ed.Params().Gx, ed.Params().Gy
	for i := 0; i < b.N; i++ {
		Gx, Gy = ed.Add(Gx, Gy, Gx, Gy)
	}
}

func BenchmarkDouble(b *testing.B) {
	ed := Ed25519()
	Gx, Gy := ed.Params().Gx, ed.Params().Gy
	for i := 0; i < b.N; i++ {
		Gx, Gy = ed.Double(Gx, Gy)
	}
}

func BenchmarkScalarBaseMult(b *testing.B) {
	ed := Ed25519()

	var k [32]byte
	_, err := io.ReadFull(rand.Reader, k[:])
	if err != nil {
		b.Fatal(err)
	}
	k[0] &= 248
	k[31] &= 127
	k[31] |= 64

	for i := 0; i < b.N; i++ {
		_, _ = ed.ScalarBaseMult(k[:])
	}
}

func BenchmarkScalarMult(b *testing.B) {
	ed := Ed25519()
	Bx, By := ed.Params().Gx, ed.Params().Gy

	var k [32]byte
	_, err := io.ReadFull(rand.Reader, k[:])
	if err != nil {
		b.Fatal(err)
	}
	k[0] &= 248
	k[31] &= 127
	k[31] |= 64

	for i := 0; i < b.N; i++ {
		_, _ = ed.ScalarMult(Bx, By, k[:])
	}
}