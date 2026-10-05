package kuznyechik

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const rfcKey = "8899aabbccddeeff0011223344556677fedcba98765432100123456789abcdef"

func TestRFC7801_L(t *testing.T) {
	cases := [][2]string{
		{"64a59400000000000000000000000000", "d456584dd0e3e84cc3166e4b7fa2890d"},
		{"d456584dd0e3e84cc3166e4b7fa2890d", "79d26221b87b584cd42fbc4ffea5de9a"},
		{"79d26221b87b584cd42fbc4ffea5de9a", "0e93691a0cfc60408b7b68f66b513c13"},
		{"0e93691a0cfc60408b7b68f66b513c13", "e6a8094fee0aa204fd97bcb0b44b8580"},
	}
	for _, c := range cases {
		in := [16]byte(unhex(t, c[0]))
		got := lTransform(in)
		if !bytes.Equal(got[:], unhex(t, c[1])) {
			t.Errorf("L(%s) = %x, want %s", c[0], got, c[1])
		}
		back := lInverse(got)
		if back != in {
			t.Errorf("L^-1(L(%s)) = %x", c[0], back)
		}
	}
}

func TestRFC7801_S(t *testing.T) {
	cases := [][2]string{
		{"ffeeddccbbaa99881122334455667700", "b66cd8887d38e8d77765aeea0c9a7efc"},
		{"b66cd8887d38e8d77765aeea0c9a7efc", "559d8dd7bd06cbfe7e7b262523280d39"},
		{"559d8dd7bd06cbfe7e7b262523280d39", "0c3322fed531e4630d80ef5c5a81c50b"},
		{"0c3322fed531e4630d80ef5c5a81c50b", "23ae65633f842d29c5df529c13f5acda"},
	}
	for _, c := range cases {
		in := unhex(t, c[0])
		out := make([]byte, 16)
		for i, v := range in {
			out[i] = pi[v]
		}
		if hex.EncodeToString(out) != c[1] {
			t.Errorf("S(%s) = %x, want %s", c[0], out, c[1])
		}
	}
}

func TestRFC7801_Constants(t *testing.T) {
	tb := getTables()
	want := map[int]string{
		1: "6ea276726c487ab85d27bd10dd849401",
		2: "dc87ece4d890f4b3ba4eb92079cbeb02",
		3: "b2259a96b4d88e0be7690430a44f7f03",
		4: "7bcd1b0b73e32ba5b79cb140f2551504",
		5: "156f6d791fab511deabb0c502fd18105",
		6: "a74af7efab73df160dd208608b9efe06",
		7: "c9e8819dc73ba5ae50f5b570561a6a07",
		8: "f6593616e6055689adfba18027aa2a08",
	}
	for i, w := range want {
		var got [16]byte
		tb.c[i-1].put(got[:])
		if hex.EncodeToString(got[:]) != w {
			t.Errorf("C_%d = %x, want %s", i, got, w)
		}
	}
}

func TestRFC7801_RoundKeys(t *testing.T) {
	c, err := NewCipher(unhex(t, rfcKey))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"8899aabbccddeeff0011223344556677",
		"fedcba98765432100123456789abcdef",
		"db31485315694343228d6aef8cc78c44",
		"3d4553d8e9cfec6815ebadc40a9ffd04",
		"57646468c44a5e28d3e59246f429f1ac",
		"bd079435165c6432b532e82834da581b",
		"51e640757e8745de705727265a0098b1",
		"5a7925017b9fdd3ed72a91a22286f984",
		"bb44e25378c73123a5f32f73cdb6e517",
		"72e9dd7416bcf45b755dbaa88e4a4043",
	}
	k := c.(*kuz)
	for i, w := range want {
		var got [16]byte
		k.rk[i].put(got[:])
		if hex.EncodeToString(got[:]) != w {
			t.Errorf("K_%d = %x, want %s", i+1, got, w)
		}
	}
}

func TestRFC7801_EncryptDecrypt(t *testing.T) {
	c, err := NewCipher(unhex(t, rfcKey))
	if err != nil {
		t.Fatal(err)
	}
	pt := unhex(t, "1122334455667700ffeeddccbbaa9988")
	ct := unhex(t, "7f679d90bebc24305a468d42b9d4edcd")
	got := make([]byte, 16)
	c.Encrypt(got, pt)
	if !bytes.Equal(got, ct) {
		t.Errorf("Encrypt = %x, want %x", got, ct)
	}
	c.Decrypt(got, ct)
	if !bytes.Equal(got, pt) {
		t.Errorf("Decrypt = %x, want %x", got, pt)
	}
	// In-place operation.
	buf := append([]byte(nil), pt...)
	c.Encrypt(buf, buf)
	if !bytes.Equal(buf, ct) {
		t.Errorf("in-place Encrypt = %x", buf)
	}
	c.Decrypt(buf, buf)
	if !bytes.Equal(buf, pt) {
		t.Errorf("in-place Decrypt = %x", buf)
	}
}

func TestRoundTripRandom(t *testing.T) {
	for n := 0; n < 200; n++ {
		key := make([]byte, KeySize)
		pt := make([]byte, BlockSize)
		rand.Read(key)
		rand.Read(pt)
		c, err := NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		ct := make([]byte, BlockSize)
		out := make([]byte, BlockSize)
		c.Encrypt(ct, pt)
		c.Decrypt(out, ct)
		if !bytes.Equal(out, pt) {
			t.Fatalf("round trip failed for key %x pt %x", key, pt)
		}
		if bytes.Equal(ct, pt) {
			t.Fatalf("ciphertext equals plaintext")
		}
	}
}

func TestKeySize(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := NewCipher(make([]byte, n)); err == nil {
			t.Errorf("key size %d accepted", n)
		}
	}
}
