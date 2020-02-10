package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestCommitObjectDeserialize(t *testing.T) {
	cases := map[string]struct {
		raw     string
		wantObj CommitObject
		wantErr error
	}{
		"empty": {
			raw: ``,
			wantObj: CommitObject{
				Header: map[string][]string{},
			},
		},
		"single key": {
			raw: `foo bar`,
			wantObj: CommitObject{
				Header: map[string][]string{"foo": []string{"bar"}},
			},
		},
		"real message": {
			raw: `tree c7aebf0cbe2b1a70501c7b7e1e28faceaba77541
parent c2367d038bac610d36342cb5e3a88b5b0ca16616
author Bob R <bobr@example.com> 1580755918 +0100
committer Bob R <bobr@example.com> 1580755918 +0100

A commit message`,

			wantObj: CommitObject{
				Header: map[string][]string{
					"tree":      []string{"c7aebf0cbe2b1a70501c7b7e1e28faceaba77541"},
					"parent":    []string{"c2367d038bac610d36342cb5e3a88b5b0ca16616"},
					"author":    []string{"Bob R <bobr@example.com> 1580755918 +0100"},
					"committer": []string{"Bob R <bobr@example.com> 1580755918 +0100"},
				},
				Comment: "A commit message",
			},
		},
	}

	for testName, tc := range cases {
		t.Run(testName, func(t *testing.T) {
			var got CommitObject
			err := got.Deserialize([]byte(tc.raw))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("unexpected error: %+v", err)
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, tc.wantObj) {
				t.Logf("want %q", tc.wantObj)
				t.Fatalf("got  %q", got)
			}
		})
	}
}
