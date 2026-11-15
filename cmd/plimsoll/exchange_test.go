package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// modern is the metadata every request of revision 2026-07-28 carries.
const modern = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

// exchange runs the program's composition in this process with the lines as its input and returns the lines it
// wrote. The input stays open until every request that carries an id is answered, as a client keeps it open; then it
// ends, and the program must end with 0. Every line it writes must be a JSON object.
func exchange(t *testing.T, lines ...string) (responses []string, stderr string) {
	t.Helper()
	var ids []string // the ids of the requests, in input order
	for _, line := range lines {
		for part := range strings.SplitSeq(line, "\n") {
			var request map[string]json.RawMessage
			if json.Unmarshal([]byte(part), &request) == nil && request["id"] != nil {
				ids = append(ids, string(request["id"]))
			}
		}
	}
	want := len(ids)
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	var errOut bytes.Buffer
	done := make(chan int, 1)
	go func() {
		code := run(context.Background(), inReader, outWriter, &errOut, os.Environ())
		_ = outWriter.Close()
		_ = inReader.Close()
		done <- code
	}()
	go func() {
		for _, line := range lines {
			if _, err := io.WriteString(inWriter, line+"\n"); err != nil {
				return
			}
		}
	}()
	got := make(chan string)
	go func() {
		defer close(got)
		scanner := bufio.NewScanner(outReader)
		scanner.Buffer(nil, 16<<20)
		for scanner.Scan() {
			got <- scanner.Text()
		}
	}()
	deadline := time.After(60 * time.Second)
	for len(responses) < want {
		select {
		case line, open := <-got:
			if !open {
				t.Fatalf("the program ended after %d of %d answers: %q", len(responses), want, responses)
			}
			responses = append(responses, line)
		case <-deadline:
			t.Fatalf("%d of %d answers within 60 s: %q", len(responses), want, responses)
		}
	}
	_ = inWriter.Close()
	for line := range got {
		responses = append(responses, line)
	}
	if code := <-done; code != 0 {
		t.Fatalf("run ended with %d: %s", code, errOut.String())
	}
	// Answers may come in any order; they are returned in the order of the requests, matched by id.
	position := func(line string) int {
		var response map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("a line of output is not a JSON object: %q", line)
		}
		if index := slices.Index(ids, string(response["id"])); index >= 0 {
			return index
		}
		return len(ids)
	}
	slices.SortStableFunc(responses, func(a, b string) int { return position(a) - position(b) })
	return responses, errOut.String()
}

// sameJSON compares two JSON texts by their decoded values.
func sameJSON(t *testing.T, got, want string) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(got), &a); err != nil {
		t.Fatalf("not JSON: %q", got)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("the expected text is not JSON: %q", want)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// decode reads one response into a map.
func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal([]byte(line), &object); err != nil {
		t.Fatalf("not JSON: %q", line)
	}
	return object
}

// as asserts a decoded JSON value's type and fails the test when it has another.
func as[T any](t *testing.T, value any) T {
	t.Helper()
	typed, ok := value.(T)
	if !ok {
		t.Fatalf("%v is a %T, want a %T", value, value, typed)
	}
	return typed
}

// at walks a decoded JSON value by object keys and array indices.
func at(t *testing.T, value any, steps ...any) any {
	t.Helper()
	for _, step := range steps {
		switch key := step.(type) {
		case string:
			value = as[map[string]any](t, value)[key]
		case int:
			list := as[[]any](t, value)
			if key >= len(list) {
				t.Fatalf("%v has no element %d", list, key)
			}
			value = list[key]
		}
	}
	return value
}
