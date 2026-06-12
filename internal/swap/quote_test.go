package swap

import (
	"context"
	"errors"
	"testing"
)

type mockProvider struct {
	out string
	err error

	gotFrom, gotTo, gotAmount string
}

func (m *mockProvider) GetPrice(_ context.Context, from, to, amount string) (string, error) {
	m.gotFrom, m.gotTo, m.gotAmount = from, to, amount
	return m.out, m.err
}

func TestQuoteGet(t *testing.T) {
	p := &mockProvider{out: "42.5"}
	q := NewQuote(p)

	got, err := q.Get(context.Background(), "0xfrom", "0xto", "1.5")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.EstimatedOutput != "42.5" {
		t.Errorf("EstimatedOutput = %q, want 42.5", got.EstimatedOutput)
	}
	if got.FromToken != "0xfrom" || got.ToToken != "0xto" || got.InputAmount != "1.5" {
		t.Errorf("echoed fields wrong: %+v", got)
	}
	if p.gotFrom != "0xfrom" || p.gotTo != "0xto" || p.gotAmount != "1.5" {
		t.Errorf("provider args wrong: %+v", p)
	}
}

func TestQuoteValidation(t *testing.T) {
	q := NewQuote(&mockProvider{out: "1"})
	bad := []struct {
		from, to, amt string
	}{
		{"", "0xto", "1"},
		{"   ", "0xto", "1"},
		{"0xfrom", "", "1"},
		{"0xfrom", "0xto", ""},
		{"0xfrom", "0xto", "abc"},
		{"0xfrom", "0xto", "0"},
		{"0xfrom", "0xto", "-5"},
	}
	for _, c := range bad {
		if _, err := q.Get(context.Background(), c.from, c.to, c.amt); err == nil {
			t.Errorf("expected error for (%q,%q,%q)", c.from, c.to, c.amt)
		}
	}
}

func TestQuoteProviderError(t *testing.T) {
	want := errors.New("boom")
	q := NewQuote(&mockProvider{err: want})
	if _, err := q.Get(context.Background(), "0xfrom", "0xto", "1"); !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
