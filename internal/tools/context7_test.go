package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agamyusliman/inferenesia-app/internal/context7"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

type stubContext7Backend struct {
	docs string
	err  error
}

func (s *stubContext7Backend) Search(ctx context.Context, libraryName, query string) ([]context7.Library, error) {
	return nil, errors.New("not implemented in stub")
}
func (s *stubContext7Backend) FetchContext(ctx context.Context, libraryID, query string) (string, error) {
	return s.docs, s.err
}
func (s *stubContext7Backend) FetchDocs(ctx context.Context, libraryName, query, libraryID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.docs, nil
}

func TestContext7Query_RegisteredAndExecuted(t *testing.T) {
	gw, err := writegate.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	reg.SetContext7Backend(&stubContext7Backend{docs: "react hooks docs"})

	defs := reg.Definitions()
	found := false
	for _, d := range defs {
		if d.Function.Name == NameContext7Query {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("context7_query not registered in Definitions")
	}

	res := reg.Execute(context.Background(), Call{
		ID:        "c1",
		Name:      NameContext7Query,
		Arguments: `{"library_name":"react","query":"useState"}`,
	}, nil)
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "react hooks docs") {
		t.Errorf("unexpected content: %q", res.Content)
	}
}

func TestContext7Query_SoftFailOnError(t *testing.T) {
	gw, err := writegate.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	reg.SetContext7Backend(&stubContext7Backend{err: errors.New("upstream down")})

	res := reg.Execute(context.Background(), Call{
		ID:        "c1",
		Name:      NameContext7Query,
		Arguments: `{"library_name":"react","query":"useState"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("expected error result on backend failure")
	}
	if !strings.Contains(res.Content, "upstream down") {
		t.Errorf("expected error message in content, got: %q", res.Content)
	}
}

func TestContext7Query_RequiresArgs(t *testing.T) {
	gw, err := writegate.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	reg.SetContext7Backend(&stubContext7Backend{})

	cases := []struct {
		name string
		args string
		want string
	}{
		{"empty library_name", `{"library_name":"","query":"x"}`, "library_name is required"},
		{"empty query", `{"library_name":"react","query":""}`, "query is required"},
		{"bad json", `{not json`, "invalid arguments JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := reg.Execute(context.Background(), Call{
				ID: "c1", Name: NameContext7Query, Arguments: tc.args,
			}, nil)
			if !res.IsError {
				t.Fatal("expected error")
			}
			if !strings.Contains(res.Content, tc.want) {
				t.Errorf("expected %q in %q", tc.want, res.Content)
			}
		})
	}
}

func TestContext7Query_NoBackend(t *testing.T) {
	gw, err := writegate.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	res := reg.Execute(context.Background(), Call{
		ID:        "c1",
		Name:      NameContext7Query,
		Arguments: `{"library_name":"react","query":"x"}`,
	}, nil)
	if !res.IsError {
		t.Fatal("expected error when no backend")
	}
}

func TestEnsureContext7Backend_AttachesClient(t *testing.T) {
	gw, err := writegate.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(gw)
	if reg.Context7Backend() != nil {
		t.Fatal("expected nil before ensure")
	}
	EnsureContext7Backend(reg)
	if reg.Context7Backend() == nil {
		t.Fatal("expected backend attached after ensure")
	}
	defs := reg.Definitions()
	found := false
	for _, d := range defs {
		if d.Function.Name == NameContext7Query {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected context7_query in definitions after ensure")
	}
}
