package research

import "testing"

func TestNaturalQuery(t *testing.T) {
	got := NaturalQuery("Lãi suất của NHNN ảnh hưởng thế nào tới AI agent?")
	want := `"lãi suất" OR "ai agent" OR "lãi" OR "suất" OR "nhnn" OR "ai" OR "agent"`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if NaturalQuery("là của và") != "" {
		t.Fatal("chỉ có stopword phải ra rỗng")
	}
}
