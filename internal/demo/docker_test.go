package demo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fleetmgr/internal/demo/demotest"
)

func testDocker(t *testing.T) (*Docker, *demotest.Engine) {
	f, sock := demotest.NewEngine(t)
	return New(Config{Socket: sock, Image: "node:test", Network: "net1", ManagerURL: "http://m:8080", ExporterAddr: "ne:9100"}), f
}

func TestCreateListAndLifecycle(t *testing.T) {
	d, f := testDocker(t)
	ctx := context.Background()
	f.Add("unrelated", "running", nil)

	n1, err := d.Create(ctx, "tok1")
	if err != nil {
		t.Fatal(err)
	}
	n2, err := d.Create(ctx, "tok2")
	if err != nil {
		t.Fatal(err)
	}
	if n1.Name != "demo-node-1" || n2.Name != "demo-node-2" {
		t.Fatalf("names %q %q", n1.Name, n2.Name)
	}
	c := f.Containers["demo-node-1"]
	if c.State != "running" || c.Network != "net1" || c.Labels[Label] != "1" {
		t.Fatalf("container %+v", c)
	}
	env := strings.Join(c.Env, " ")
	for _, want := range []string{"FLEET_ENROLL_TOKEN=tok1", "FLEET_MANAGER_URL=http://m:8080", "FLEET_NODE_EXPORTER_ADDR=ne:9100"} {
		if !strings.Contains(env, want) {
			t.Fatalf("env %q missing %q", env, want)
		}
	}

	list, err := d.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list should only hold labelled containers: %+v", list)
	}

	if err := d.Stop(ctx, "demo-node-1"); err != nil {
		t.Fatal(err)
	}
	if err := d.Stop(ctx, "demo-node-1"); err != nil {
		t.Fatalf("stopping twice should be fine: %v", err)
	}
	if f.Containers["demo-node-1"].State != "exited" {
		t.Fatal("not stopped")
	}
	if err := d.Start(ctx, "demo-node-1"); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove(ctx, "demo-node-2"); err != nil {
		t.Fatal(err)
	}
	if n3, _ := d.Create(ctx, "tok3"); n3.Name != "demo-node-2" {
		t.Fatalf("next name after removal: %q", n3.Name)
	}
}

func TestRefusesUnlabelledContainers(t *testing.T) {
	d, f := testDocker(t)
	f.Add("postgres", "running", map[string]string{"other": "1"})
	for _, op := range []func(context.Context, string) error{d.Stop, d.Start, d.Remove} {
		if err := op(context.Background(), "postgres"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	}
	if f.Containers["postgres"].State != "running" {
		t.Fatal("unlabelled container was touched")
	}
	for _, c := range f.Calls {
		if strings.HasPrefix(c, "POST /containers/postgres") || strings.HasPrefix(c, "DELETE /containers/postgres") {
			t.Fatalf("made a call against an unlabelled container: %s", c)
		}
	}
}

func TestMissingImageMessage(t *testing.T) {
	d, f := testDocker(t)
	f.Images = map[string]bool{}
	_, err := d.Create(context.Background(), "tok")
	if err == nil || !strings.Contains(err.Error(), "docker compose up --build") {
		t.Fatalf("want a hint about building the image, got %v", err)
	}
}

func TestNextName(t *testing.T) {
	if got := NextName(nil); got != "demo-node-1" {
		t.Fatal(got)
	}
	if got := NextName([]Node{{Name: "demo-node-2"}, {Name: "demo-node-10"}}); got != "demo-node-11" {
		t.Fatal(got)
	}
}
