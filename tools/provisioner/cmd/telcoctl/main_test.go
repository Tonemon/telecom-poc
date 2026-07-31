package main

import (
	"reflect"
	"testing"
)

// A single-character value (e.g. "--count 2") must be consumed as the flag's
// value, not mistaken for a boolean flag and left in rest.
func TestPopFlag_SingleCharValue(t *testing.T) {
	val, present, rest := popFlag([]string{"--count", "2", "--apn", "internet"}, "count")
	if !present {
		t.Fatal("count should be present")
	}
	if val != "2" {
		t.Fatalf("count value = %q, want %q", val, "2")
	}
	if want := []string{"--apn", "internet"}; !reflect.DeepEqual(rest, want) {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
}

// A flag with no following value (or followed by another flag) stays boolean.
func TestPopFlag_BooleanFlag(t *testing.T) {
	val, present, rest := popFlag([]string{"1.2.3.4", "--clear"}, "clear")
	if !present {
		t.Fatal("clear should be present")
	}
	if val != "" {
		t.Fatalf("clear value = %q, want empty", val)
	}
	if want := []string{"1.2.3.4"}; !reflect.DeepEqual(rest, want) {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
}

// A dash-prefixed token after a flag is not swallowed as its value.
func TestPopFlag_NextFlagNotConsumed(t *testing.T) {
	val, present, rest := popFlag([]string{"--note", "--reason", "BATCH"}, "note")
	if !present {
		t.Fatal("note should be present")
	}
	if val != "" {
		t.Fatalf("note value = %q, want empty", val)
	}
	if want := []string{"--reason", "BATCH"}; !reflect.DeepEqual(rest, want) {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
}
