package storage

import "testing"

// posts that differ only in whitespace, case, emoji, or links normalize to the same hash
func TestDedupHash_IgnoresNoise(t *testing.T) {
	base := "Backend Go developer needed, remote OK"
	variants := []string{
		"backend go developer needed, remote ok",
		"Backend   Go   developer needed,   remote OK",
		"Backend Go developer needed, remote OK 🚀🚀",
		"Backend Go developer needed, remote OK https://t.me/somejobchan",
		"Backend Go developer needed, remote OK\n\n@recruiter",
	}
	want := dedupHash(base)
	if want == "" {
		t.Fatal("base hash is empty")
	}
	for _, v := range variants {
		if got := dedupHash(v); got != want {
			t.Errorf("expected %q to hash equal to base, got different hash", v)
		}
	}
}

// genuinely different posts hash differently
func TestDedupHash_DistinctText(t *testing.T) {
	a := dedupHash("Go backend role")
	b := dedupHash("Rust systems role")
	if a == b {
		t.Fatal("expected different text to produce different hashes")
	}
}

// text that is only punctuation/emoji normalizes to empty and is never a duplicate
func TestDedupHash_EmptyAfterNormalize(t *testing.T) {
	if h := dedupHash("🚀🚀🚀 !!! ---"); h != "" {
		t.Fatalf("expected empty hash for noise-only text, got %q", h)
	}
}
