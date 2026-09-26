// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/google/ax/internal/guest"
	"github.com/google/ax/internal/tunnel"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd, cleanArgs, atespace, explicitServer, kubeContext, axNamespace, atespaceExplicit, parseErr := parseGlobalArgs(os.Args[1:])
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", parseErr)
		os.Exit(1)
	}

	if cmd == "" {
		printUsage()
		os.Exit(1)
	}

	// Commands that don't require an AX server connection
	switch cmd {
	case "version":
		fmt.Println(axVersionLine())
		return
	case "help", "-h", "--help":
		printUsage()
		return
	case "ctx", "context":
		if err := runContext(kubeContext, cleanArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	case "tunnel":
		if err := runTunnel(cleanArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Resolve the AX server URL (auto-tunneling to active kube context if not explicitly set)
	serverURL, err := tunnel.EnsureServerURL(tunnel.Options{
		ServerURL: explicitServer,
		Context:   kubeContext,
		Namespace: axNamespace,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch cmd {
	case "apply":
		err = runApply(serverURL, atespace, atespaceExplicit, cleanArgs)
	case "get":
		err = runGet(serverURL, atespace, cleanArgs)
	case "describe":
		err = runDescribe(serverURL, atespace, cleanArgs)
	case "watch":
		err = runWatch(serverURL, atespace, cleanArgs)
	case "suspend":
		err = runSuspend(serverURL, atespace, cleanArgs)
	case "resume":
		err = runResume(serverURL, atespace, cleanArgs)
	case "delete":
		err = runDelete(serverURL, atespace, cleanArgs)
	case "ssh":
		err = runSSH(serverURL, atespace, kubeContext, cleanArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// parseGlobalArgs splits the command line into the ax command, the remaining
// positional args, and the global flags (--atespace, --server, --context,
// --namespace). A bare "--" ends option parsing: everything after it is
// positional, even if it looks like a global flag, so the remote command in
// `ax ssh mytask -- env --server` reaches the guest intact instead of being
// swallowed by the global parser.
func parseGlobalArgs(args []string) (cmd string, cleanArgs []string, atespace, explicitServer, kubeContext, axNamespace string, atespaceExplicit bool, err error) {
	atespace = "default"
	axNamespace = "ax-system"
	noMoreFlags := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !noMoreFlags && arg == "--" {
			noMoreFlags = true
			// A "--" before the command is just a separator: `ax -- get
			// tasks` still runs get. It is dropped from cleanArgs so it
			// cannot land in the kind position; a "--" after the command
			// is kept (ssh strips its own leading separator).
			if cmd != "" {
				cleanArgs = append(cleanArgs, arg)
			}
			continue
		}
		if noMoreFlags {
			// The command word is still the command after "--": `ax -- get
			// tasks` used to report "unknown command" because this branch
			// never captured it. Only the first bare word wins; the "--"
			// itself is dropped from cleanArgs so it cannot land in the kind
			// position; a "--" after the command is kept (ssh strips its own
			// leading separator).
			if cmd == "" && !strings.HasPrefix(arg, "-") {
				cmd = arg
				continue
			}
			cleanArgs = append(cleanArgs, arg)
			continue
		}
		if arg == "-a" || arg == "--atespace" {
			if i+1 < len(args) {
				atespace = args[i+1]
				atespaceExplicit = true
				i++
			} else {
				err = fmt.Errorf("flag %s requires a value", arg)
				return
			}
		} else if strings.HasPrefix(arg, "--atespace=") {
			atespace = strings.TrimPrefix(arg, "--atespace=")
			atespaceExplicit = true
		} else if strings.HasPrefix(arg, "-a=") {
			// The short = form (`-a=prod`) that Go's flag package and kubectl
			// accept: the old code only matched bare `-a`, so `-a=prod` fell
			// into cleanArgs and blew up in normalizeKind with a misleading
			// "unsupported kind" instead of scoping the command.
			atespace = strings.TrimPrefix(arg, "-a=")
			atespaceExplicit = true
		} else if arg == "--server" {
			if i+1 < len(args) {
				explicitServer = args[i+1]
				i++
			} else {
				err = fmt.Errorf("flag %s requires a value", arg)
				return
			}
		} else if strings.HasPrefix(arg, "--server=") {
			explicitServer = strings.TrimPrefix(arg, "--server=")
		} else if arg == "--context" {
			if i+1 < len(args) {
				kubeContext = args[i+1]
				i++
			} else {
				err = fmt.Errorf("flag %s requires a value", arg)
				return
			}
		} else if strings.HasPrefix(arg, "--context=") {
			kubeContext = strings.TrimPrefix(arg, "--context=")
		} else if arg == "-n" || arg == "--namespace" {
			if i+1 < len(args) {
				axNamespace = args[i+1]
				i++
			} else {
				err = fmt.Errorf("flag %s requires a value", arg)
				return
			}
		} else if strings.HasPrefix(arg, "--namespace=") {
			axNamespace = strings.TrimPrefix(arg, "--namespace=")
		} else if strings.HasPrefix(arg, "-n=") {
			// Same short-= gap as -a=: `-n=prod` fell into cleanArgs.
			axNamespace = strings.TrimPrefix(arg, "-n=")
		} else if cmd == "" && !strings.HasPrefix(arg, "-") {
			cmd = arg
		} else {
			cleanArgs = append(cleanArgs, arg)
		}
	}
	return
}

// axVersionLine is the `ax version` output. The CLI is a gRPC client
// against an AX server, not a bundled engine: the old "standalone redis
// engine" line predated the client-server restructure and told users
// debugging connection issues the opposite of the truth.
func axVersionLine() string {
	return "ax version v1alpha1 (AX CLI)"
}

func printUsage() {
	fmt.Println(`AX CLI - Autonomous agent execution control

Usage:
  ax [command] [flags]

Available Commands:
  apply -f <file>         Apply resources (tasks, gateways, workspaces, models) from a file or stdin
  get tasks               List tasks
  get task <name>         Get a specific task
  get gateways            List gateways
  get gateway <name>      Get a specific gateway
  get workspaces          List workspaces
  get workspace <name>    Get a specific workspace
  get models              List models
  get model <name>        Get a specific model
  describe task <name>    Show detailed information about a task
  describe gateway <name> Show detailed information about a gateway
  describe workspace <name> Show detailed information about a workspace
  describe model <name>   Show detailed information about a model
  watch task <name>       Stream live status and condition updates for a task
  ssh <task-name> [-- cmd] Run a command or shell inside the running task container
  suspend task <name>     Suspend execution of a task and checkpoint state
  resume task <name>      Resume execution of a suspended task
  delete task <name>      Delete a task
  delete gateway <name>   Delete a gateway
  delete workspace <name> Delete a workspace
  delete model <name>     Delete a model
  ctx, context            Show active Kubernetes context and AX connection
  tunnel <list|stop>      Manage background tunnels to Kubernetes clusters
  version                 Print AX version

Flags:
  -a, --atespace string       Task atespace scope (default: "default")
  -n, --namespace string      Kubernetes namespace where AX is installed (default: "ax-system")
  --context string            Kubernetes context (defaults to active kubectx / current-context)
  --server string             AX API server address (default: auto-detected from kube context or $AX_SERVER)`)
}

// normalizeServerURL trims accidental whitespace and trailing slashes from
// a --server value or AX_SERVER export (copy-paste leaves "http://host:8080/"
// behind). A trailing slash makes the gRPC target unresolvable and surfaces
// as a confusing DNS error instead of a clean dial.
func normalizeServerURL(serverURL string) string {
	s := strings.TrimSpace(serverURL)
	for len(s) > 0 && strings.HasSuffix(s, "/") {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}

func getAXClient(serverURL string) (v1alpha1.AXClient, *grpc.ClientConn, error) {
	serverURL = normalizeServerURL(serverURL)
	target := strings.TrimPrefix(serverURL, "http://")
	target = strings.TrimPrefix(target, "https://")
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to %s: %w", target, err)
	}
	return v1alpha1.NewAXClient(conn), conn, nil
}

func runApply(serverURL, atespace string, atespaceExplicit bool, args []string) error {
	data, ok, err := manifestFromArgs(args)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing required flag: -f <file>")
	}

	// apply takes no positional args: anything besides -f/--file and its
	// value is a typo (`ax apply -f foo.yaml bar` applied the manifest and
	// silently dropped "bar"). Same defect shape as get/describe/delete/
	// watch/tunnel, so it gets the same "unexpected extra argument" usage
	// error. Checked before dialing, like the other commands.
	if err := validateApplyPositionals(args); err != nil {
		return err
	}

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return applyManifests(ctx, client, data, atespace, atespaceExplicit)
}

// validateApplyPositionals rejects stray positional args for `ax apply`:
// the command takes only -f/--file, so anything else is a typo. The -f
// flag value and other flags pass through untouched.
func validateApplyPositionals(args []string) error {
	seenFile := false
	for i := 0; i < len(args); i++ {
		if _, isFile := fileFlagValue(args[i]); isFile {
			// manifestFromArgs applies only the FIRST -f/--file, so a second
			// one was silently dropped (`ax apply -f a.yaml -f b.yaml`
			// applied only a.yaml with exit 0). Make that loud instead.
			if seenFile {
				return fmt.Errorf("usage: ax apply -f <file> (duplicate -f/--file flag; specify one manifest)")
			}
			seenFile = true
			if args[i] == "-f" || args[i] == "--file" {
				i++ // skip the flag value
			}
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			continue // other flags pass through untouched
		}
		return fmt.Errorf("usage: ax apply -f <file> (unexpected extra argument %q)", args[i])
	}
	return nil
}

// applyManifests applies each document of a multi-document manifest in order.
// Document indexes in error messages are 1-based and count only real
// documents: empty documents (a stray "---") do not shift the numbering.
// A failure in one document aborts the apply and names the failing document;
// documents applied before the failure are already reported.
// atespace/atespaceExplicit are the CLI --atespace flag and whether the user
// passed it: an explicit flag wins over the manifest's metadata.atespace
// (kubectl parity), otherwise the manifest's atespace stands.
func applyManifests(ctx context.Context, client v1alpha1.AXClient, data []byte, atespace string, atespaceExplicit bool) error {
	// Manifests are parsed here and submitted one resource at a time through the
	// typed RPCs; the server never sees raw YAML.
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	docIndex := 0
	for {
		var doc yaml.Node
		if err := decoder.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decoding document %d: %w", docIndex+1, err)
		}
		if doc.Kind == 0 || (doc.Kind == yaml.DocumentNode && len(doc.Content) == 0) {
			continue // empty document, e.g. a trailing "---"; not counted
		}
		if isEmptyDocument(&doc) {
			continue // bare "---" separator decodes to a null node, not an empty document
		}
		docIndex++

		kind, name, outcome, err := applyDocument(ctx, client, &doc, atespace, atespaceExplicit)
		if err != nil {
			return fmt.Errorf("applying document %d: %w", docIndex, err)
		}
		fmt.Printf("%s.ax.io/%s %s\n", strings.ToLower(kind), name, outcome)
	}
}

// isEmptyDocument reports whether a decoded YAML document carries no
// content. A bare "---" separator between documents decodes to a document
// node holding a single null scalar — not to an empty document node — so the
// naive len(Content)==0 check misses it.
func isEmptyDocument(doc *yaml.Node) bool {
	if doc.Kind == 0 {
		return true
	}
	if doc.Kind != yaml.DocumentNode {
		return false
	}
	if len(doc.Content) == 0 {
		return true
	}
	return len(doc.Content) == 1 &&
		doc.Content[0].Kind == yaml.ScalarNode &&
		doc.Content[0].Tag == "!!null"
}

// applyDocument decodes one manifest by its kind and submits it with the matching
// Update RPC. It reports the kind, the resource name, and whether the resource was
// created, configured (spec changed), or unchanged, in the style of kubectl apply.
func applyDocument(ctx context.Context, client v1alpha1.AXClient, doc *yaml.Node, atespace string, atespaceExplicit bool) (kind, name, outcome string, err error) {
	var head struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name     string `yaml:"name"`
			Atespace string `yaml:"atespace"`
		} `yaml:"metadata"`
	}
	if err := doc.Decode(&head); err != nil {
		return "", "", "", fmt.Errorf("reading kind: %w", err)
	}

	// Manifest kinds are normalized the same way user-typed kinds are for
	// get/describe/delete: "task", "Task" and "tasks" all mean the Task
	// kind. Requiring the exact canonical spelling here rejected manifests
	// that every other command accepts.
	normKind, err := normalizeKind(head.Kind)
	if err != nil {
		return "", "", "", err
	}
	kind = normKind

	// A document without metadata.name used to sail through to the server:
	// GetTask("") missed, UpdateTask persisted a nameless record, and the
	// CLI printed `task.ax.io/ created` for a resource with no name.
	// kubectl rejects this client-side; so do we, before any RPC.
	// Whitespace-only names (" ") are the same defect one character wider:
	// the == "" check passes, GetTask(" ") misses, and a record named " "
	// is persisted and printed as `task.ax.io/  created`.
	if strings.TrimSpace(head.Metadata.Name) == "" {
		return "", "", "", fmt.Errorf("missing metadata.name")
	}

	// The atespace the document lands in. apply used to route purely on the
	// manifest's metadata.atespace and never saw the CLI --atespace flag at
	// all (main never passed it), so `ax apply --atespace=prod -f task.yaml`
	// silently landed the task in the manifest's atespace (or "default").
	// kubectl parity: an explicit --atespace wins, otherwise the manifest's
	// own atespace stands, otherwise the server's "default" defaulting.
	effAtespace := atespace
	if !atespaceExplicit && head.Metadata.Atespace != "" {
		effAtespace = head.Metadata.Atespace
	}

	switch kind {
	case v1alpha1.KindTask:
		var task v1alpha1.Task
		if err := doc.Decode(&task); err != nil {
			return "", "", "", err
		}
		task.Metadata = applyDocMetadata(task.Metadata, effAtespace)
		existing, err := client.GetTask(ctx, &v1alpha1.GetTaskRequest{Atespace: effAtespace, Name: task.GetMetadata().GetName()})
		outcome, err := applyOutcome(err, existing.GetSpec(), task.GetSpec())
		if err != nil {
			return "", "", "", err
		}
		res, err := client.UpdateTask(ctx, &v1alpha1.UpdateTaskRequest{Task: &task})
		return kind, res.GetMetadata().GetName(), outcome, err

	case v1alpha1.KindGateway:
		var gw v1alpha1.Gateway
		if err := doc.Decode(&gw); err != nil {
			return "", "", "", err
		}
		gw.Metadata = applyDocMetadata(gw.Metadata, effAtespace)
		existing, err := client.GetGateway(ctx, &v1alpha1.GetGatewayRequest{Atespace: effAtespace, Name: gw.GetMetadata().GetName()})
		outcome, err := applyOutcome(err, existing.GetSpec(), gw.GetSpec())
		if err != nil {
			return "", "", "", err
		}
		res, err := client.UpdateGateway(ctx, &v1alpha1.UpdateGatewayRequest{Gateway: &gw})
		return kind, res.GetMetadata().GetName(), outcome, err

	case v1alpha1.KindWorkspace:
		var ws v1alpha1.Workspace
		if err := doc.Decode(&ws); err != nil {
			return "", "", "", err
		}
		ws.Metadata = applyDocMetadata(ws.Metadata, effAtespace)
		existing, err := client.GetWorkspace(ctx, &v1alpha1.GetWorkspaceRequest{Atespace: effAtespace, Name: ws.GetMetadata().GetName()})
		outcome, err := applyOutcome(err, existing.GetSpec(), ws.GetSpec())
		if err != nil {
			return "", "", "", err
		}
		res, err := client.UpdateWorkspace(ctx, &v1alpha1.UpdateWorkspaceRequest{Workspace: &ws})
		return kind, res.GetMetadata().GetName(), outcome, err

	case v1alpha1.KindModel:
		var m v1alpha1.Model
		if err := doc.Decode(&m); err != nil {
			return "", "", "", err
		}
		m.Metadata = applyDocMetadata(m.Metadata, effAtespace)
		existing, err := client.GetModel(ctx, &v1alpha1.GetModelRequest{Atespace: effAtespace, Name: m.GetMetadata().GetName()})
		outcome, err := applyOutcome(err, existing.GetSpec(), m.GetSpec())
		if err != nil {
			return "", "", "", err
		}
		res, err := client.UpdateModel(ctx, &v1alpha1.UpdateModelRequest{Model: &m})
		return kind, res.GetMetadata().GetName(), outcome, err

	default:
		// Unreachable: normalizeKind rejects unknown and empty kinds above,
		// but the switch must stay exhaustive.
		return "", "", "", fmt.Errorf("unsupported kind %q", head.Kind)
	}
}

// applyDocMetadata pins a decoded manifest resource to its effective atespace
// before any RPC: the existence lookup and the update must agree, and the
// update must not carry a different atespace than the one the CLI flag (or
// the manifest) selected.
func applyDocMetadata(meta *v1alpha1.ObjectMeta, atespace string) *v1alpha1.ObjectMeta {
	if meta == nil {
		meta = &v1alpha1.ObjectMeta{}
	}
	meta.Atespace = atespace
	return meta
}

// applyOutcome classifies an apply from the result of looking up the existing
// resource: "created" when it did not exist, "unchanged" when its spec already
// matches, and "configured" otherwise. Lookup failures other than NotFound are
// returned as errors.
func applyOutcome(lookupErr error, existingSpec, newSpec proto.Message) (string, error) {
	if lookupErr != nil {
		if status.Code(lookupErr) == codes.NotFound {
			return "created", nil
		}
		return "", fmt.Errorf("looking up existing resource: %w", lookupErr)
	}
	if proto.Equal(existingSpec, newSpec) {
		return "unchanged", nil
	}
	return "configured", nil
}

func runGet(serverURL, atespace string, args []string) error {
	// Validate the args before dialing: describe/watch/delete all reject
	// bad usage locally, but get used to open a connection first, so
	// `ax get bogus-kind` against a dead server reported a dial/RPC error
	// that hid the real usage problem.
	if _, err := validateGetArgs(args); err != nil {
		return err
	}

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return runGetWithClient(ctx, client, atespace, args)
}

// validateGetArgs checks the get argument shape (resource word plus an
// optional name) without touching the network. runGet calls it before
// dialing; runGetWithClient calls it again so the dial-free core keeps its
// own validation when driven with a fake client.
func validateGetArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("specify resource to get (e.g. 'ax get tasks' or 'ax get task <name>')")
	}

	// The resource position is normalized like describe/watch/delete/apply:
	// " task", "Task" and "tasks" all route to the task kind. Matching on the
	// raw lowercased word left `ax get " task"` failing with "unknown
	// resource" while every sibling command accepted it.
	resource, err := normalizeKind(args[0])
	if err != nil {
		return "", err
	}

	if len(args) > 2 {
		switch resource {
		case v1alpha1.KindTask:
			return "", fmt.Errorf("usage: ax get <task|tasks> <name> (unexpected extra argument %q)", args[2])
		case v1alpha1.KindGateway:
			return "", fmt.Errorf("usage: ax get <gateway|gateways> <name> (unexpected extra argument %q)", args[2])
		case v1alpha1.KindWorkspace:
			return "", fmt.Errorf("usage: ax get <workspace|workspaces> <name> (unexpected extra argument %q)", args[2])
		case v1alpha1.KindModel:
			return "", fmt.Errorf("usage: ax get <model|models> <name> (unexpected extra argument %q)", args[2])
		}
	}
	return resource, nil
}

// runGetWithClient is the dial-free core of runGet, so tests can drive it
// with a fake client.
func runGetWithClient(ctx context.Context, client v1alpha1.AXClient, atespace string, args []string) error {
	resource, err := validateGetArgs(args)
	if err != nil {
		return err
	}

	if resource == v1alpha1.KindTask && len(args) == 1 {
		resp, err := client.ListTasks(ctx, &v1alpha1.ListTasksRequest{Atespace: atespace})
		if err != nil {
			return fmt.Errorf("listing tasks: %w", err)
		}

		tasks := sortByName(resp.Tasks, func(t *v1alpha1.Task) string { return objectName(t.Metadata) })
		if len(tasks) == 0 {
			fmt.Println(emptyListMessage("tasks", atespace))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tATESPACE\tPHASE\tACTOR\tWORKER-IP\tAGE")
		for _, t := range tasks {
			workerIP := ""
			actor := ""
			phase := "Pending"
			if t.Status != nil {
				workerIP = t.Status.WorkerIp
				actor = t.Status.Actor
				phase = displayPhase(t.Status.Phase)
			}
			workerIP = displayWorkerIP(workerIP)
			actor = displayActor(actor)
			age := "<unknown>"
			name := ""
			tAtespace := ""
			if t.Metadata != nil {
				// Names carry no control-character validation server-side;
				// sanitize so a crafted name cannot break the table.
				name = sanitizeCell(t.Metadata.Name)
				tAtespace = sanitizeCell(t.Metadata.Atespace)
				if t.Metadata.CreationTimestamp != nil {
					age = formatAge(time.Since(t.Metadata.CreationTimestamp.AsTime()))
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				name,
				tAtespace,
				phase,
				actor,
				workerIP,
				age,
			)
		}
		return w.Flush()
	}

	if resource == v1alpha1.KindTask && len(args) == 2 {
		name := args[1]
		task, err := client.GetTask(ctx, &v1alpha1.GetTaskRequest{Atespace: atespace, Name: name})
		if err != nil {
			return fmt.Errorf("getting task %q: %w", name, err)
		}

		return yaml.NewEncoder(os.Stdout).Encode(task)
	}

	if resource == v1alpha1.KindGateway && len(args) == 1 {
		resp, err := client.ListGateways(ctx, &v1alpha1.ListGatewaysRequest{Atespace: atespace})
		if err != nil {
			return fmt.Errorf("listing gateways: %w", err)
		}

		gateways := sortByName(resp.Gateways, func(g *v1alpha1.Gateway) string { return objectName(g.Metadata) })
		if len(gateways) == 0 {
			fmt.Println(emptyListMessage("gateways", atespace))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tATESPACE\tLISTENERS\tEGRESS-HOSTS")
		for _, g := range gateways {
			var listenerList []string
			if g.Spec != nil {
				for _, l := range g.Spec.Listeners {
					s := fmt.Sprintf("%d", l.Port)
					if p := listenerProtocol(l); p != "" {
						s = fmt.Sprintf("%d/%s", l.Port, p)
					}
					listenerList = append(listenerList, s)
				}
			}
			listenersStr := strings.Join(listenerList, ",")
			if listenersStr == "" {
				listenersStr = "<none>"
			}

			egressStr := egressHostsLabel(g.GetSpec().GetEgress().GetAllowlist().GetHosts())
			if egressStr == "" {
				egressStr = "<none>"
			}

			name := ""
			gwAtespace := ""
			if g.Metadata != nil {
				name = sanitizeCell(g.Metadata.Name)
				gwAtespace = sanitizeCell(g.Metadata.Atespace)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				name,
				gwAtespace,
				listenersStr,
				egressStr,
			)
		}
		return w.Flush()
	}

	if resource == v1alpha1.KindGateway && len(args) == 2 {
		name := args[1]
		gw, err := client.GetGateway(ctx, &v1alpha1.GetGatewayRequest{Atespace: atespace, Name: name})
		if err != nil {
			return fmt.Errorf("getting gateway %q: %w", name, err)
		}

		return yaml.NewEncoder(os.Stdout).Encode(gw)
	}

	if resource == v1alpha1.KindWorkspace && len(args) == 1 {
		resp, err := client.ListWorkspaces(ctx, &v1alpha1.ListWorkspacesRequest{Atespace: atespace})
		if err != nil {
			return fmt.Errorf("listing workspaces: %w", err)
		}

		workspaces := sortByName(resp.Workspaces, func(w *v1alpha1.Workspace) string { return objectName(w.Metadata) })
		if len(workspaces) == 0 {
			fmt.Println(emptyListMessage("workspaces", atespace))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tATESPACE\tGIT-REPOS\tMCP-SERVERS")
		for _, ws := range workspaces {
			gitCount := "0"
			mcpCount := "0"
			if ws.Spec != nil {
				gitCount = fmt.Sprintf("%d", len(ws.Spec.Git))
				if ws.Spec.Mcp != nil {
					mcpCount = fmt.Sprintf("%d", len(ws.Spec.Mcp.Servers))
				}
			}
			name := ""
			wsAtespace := ""
			if ws.Metadata != nil {
				name = sanitizeCell(ws.Metadata.Name)
				wsAtespace = sanitizeCell(ws.Metadata.Atespace)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				name,
				wsAtespace,
				gitCount,
				mcpCount,
			)
		}
		return w.Flush()
	}

	if resource == v1alpha1.KindWorkspace && len(args) == 2 {
		name := args[1]
		ws, err := client.GetWorkspace(ctx, &v1alpha1.GetWorkspaceRequest{Atespace: atespace, Name: name})
		if err != nil {
			return fmt.Errorf("getting workspace %q: %w", name, err)
		}

		return yaml.NewEncoder(os.Stdout).Encode(ws)
	}

	if resource == v1alpha1.KindModel && len(args) == 1 {
		resp, err := client.ListModels(ctx, &v1alpha1.ListModelsRequest{Atespace: atespace})
		if err != nil {
			return fmt.Errorf("listing models: %w", err)
		}

		models := sortByName(resp.Models, func(m *v1alpha1.Model) string { return objectName(m.Metadata) })
		if len(models) == 0 {
			fmt.Println(emptyListMessage("models", atespace))
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 8, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tATESPACE\tPROVIDER\tMODEL")
		for _, m := range models {
			name := ""
			mAtespace := ""
			if m.Metadata != nil {
				name = sanitizeCell(m.Metadata.Name)
				mAtespace = sanitizeCell(m.Metadata.Atespace)
			}
			provider := ""
			modelName := ""
			if m.Spec != nil {
				provider = sanitizeCell(m.Spec.Provider)
				modelName = sanitizeCell(m.Spec.Model)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				name,
				mAtespace,
				provider,
				modelName,
			)
		}
		return w.Flush()
	}

	if resource == v1alpha1.KindModel && len(args) == 2 {
		name := args[1]
		m, err := client.GetModel(ctx, &v1alpha1.GetModelRequest{Atespace: atespace, Name: name})
		if err != nil {
			return fmt.Errorf("getting model %q: %w", name, err)
		}

		return yaml.NewEncoder(os.Stdout).Encode(m)
	}

	return fmt.Errorf("unknown resource %q", resource) // unreachable: normalizeKind rejects unknown kinds above
}

// emptyListMessage is the "nothing to show" line for the get list views.
// kubectl prints "No resources found"; ax's own tunnel list prints its own
// empty line — a bare table header on an empty atespace is ambiguous about
// whether the list even ran.
func emptyListMessage(resource, atespace string) string {
	return fmt.Sprintf("No %s found in atespace %q.", resource, atespace)
}

// formatCommandLine renders a command argv slice the way a user would type
// it. Printing the slice with %v shows Go syntax ("[a b]") in describe
// output; joining with spaces shows the actual command line.
func formatCommandLine(argv []string) string {
	return strings.Join(argv, " ")
}

func runDescribe(serverURL, atespace string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ax describe <task|gateway|workspace|model> <name>")
	}
	if len(args) > 2 {
		return fmt.Errorf("usage: ax describe <task|gateway|workspace|model> <name> (unexpected extra argument %q)", args[2])
	}
	kind, err := normalizeKind(args[0])
	if err != nil {
		return err
	}
	name := args[1]

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if kind == v1alpha1.KindModel {
		m, err := client.GetModel(ctx, &v1alpha1.GetModelRequest{Atespace: atespace, Name: name})
		if err != nil {
			return fmt.Errorf("getting model %q: %w", name, err)
		}

		mName := ""
		mAtespace := ""
		if m.Metadata != nil {
			mName = m.Metadata.Name
			mAtespace = m.Metadata.Atespace
		}
		fmt.Printf("Name:               %s\n", mName)
		fmt.Printf("Atespace:           %s\n", mAtespace)
		if m.Spec != nil {
			fmt.Printf("Provider:           %s\n", m.Spec.Provider)
			fmt.Printf("Model:              %s\n", m.Spec.Model)
			if params := m.Spec.GetParameters().AsMap(); len(params) > 0 {
				fmt.Println("Parameters:")
				keys := make([]string, 0, len(params))
				for k := range params {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("  %s: %s\n", k, formatParamValue(params[k]))
				}
			}
			if m.Spec.SecretKey != nil {
				if m.Spec.SecretKey.Name != "" && m.Spec.SecretKey.Key != "" && m.Spec.SecretKey.Name != m.Spec.SecretKey.Key {
					fmt.Printf("Secret Key:         %s (key: %s)\n", m.Spec.SecretKey.Name, m.Spec.SecretKey.Key)
				} else if m.Spec.SecretKey.Key != "" {
					fmt.Printf("Secret Key:         %s\n", m.Spec.SecretKey.Key)
				} else if m.Spec.SecretKey.Name != "" {
					fmt.Printf("Secret Key:         %s\n", m.Spec.SecretKey.Name)
				}
			}
		}
		return nil
	}

	if kind == v1alpha1.KindWorkspace {
		ws, err := client.GetWorkspace(ctx, &v1alpha1.GetWorkspaceRequest{Atespace: atespace, Name: name})
		if err != nil {
			return fmt.Errorf("getting workspace %q: %w", name, err)
		}

		wsName := ""
		wsAtespace := ""
		if ws.Metadata != nil {
			wsName = ws.Metadata.Name
			wsAtespace = ws.Metadata.Atespace
		}
		fmt.Printf("Name:         %s\n", wsName)
		fmt.Printf("Atespace:     %s\n", wsAtespace)
		if ws.Spec != nil {
			if len(ws.Spec.Git) > 0 {
				fmt.Println("Git Repositories:")
				for _, g := range ws.Spec.Git {
					branch := displayBranch(g.Branch)
					fmt.Printf("  - %s (%s, branch: %s)\n", g.Name, g.Repo, branch)
				}
			}
			if ws.Spec.Mcp != nil {
				if len(ws.Spec.Mcp.Registries) > 0 {
					fmt.Println("MCP Registries:")
					for _, r := range ws.Spec.Mcp.Registries {
						fmt.Printf("  - Provider: %s, Query: %q\n", r.Provider, r.Query)
					}
				}
				if len(ws.Spec.Mcp.Servers) > 0 {
					fmt.Println("MCP Servers:")
					for _, s := range ws.Spec.Mcp.Servers {
						if s.Endpoint != "" {
							fmt.Printf("  - %s: %s\n", s.Name, s.Endpoint)
						} else {
							cmdLine := s.Command
							if argLine := formatCommandLine(s.Args); argLine != "" {
								cmdLine += " " + argLine
							}
							fmt.Printf("  - %s: %s\n", s.Name, cmdLine)
						}
					}
				}
			}
			if ws.Spec.Skills != nil {
				if len(ws.Spec.Skills.Registries) > 0 {
					fmt.Println("Skill Registries:")
					for _, r := range ws.Spec.Skills.Registries {
						fmt.Printf("  - Provider: %s, Query: %q\n", r.Provider, r.Query)
					}
				}
				if ws.Spec.Skills.Path != "" {
					fmt.Printf("Skills Path:      %s\n", ws.Spec.Skills.Path)
				}
			}
		}
		return nil
	}

	if kind == v1alpha1.KindGateway {
		gw, err := client.GetGateway(ctx, &v1alpha1.GetGatewayRequest{Atespace: atespace, Name: name})
		if err != nil {
			return fmt.Errorf("getting gateway %q: %w", name, err)
		}

		gwName := ""
		gwAtespace := ""
		if gw.Metadata != nil {
			gwName = gw.Metadata.Name
			gwAtespace = gw.Metadata.Atespace
		}
		fmt.Printf("Name:         %s\n", gwName)
		fmt.Printf("Atespace:     %s\n", gwAtespace)
		if gw.Spec != nil {
			if len(gw.Spec.Listeners) > 0 {
				fmt.Println("Listeners:")
				for _, l := range gw.Spec.Listeners {
					if p := listenerProtocol(l); p != "" {
						fmt.Printf("  - %s: %d (%s)\n", l.Name, l.Port, p)
					} else {
						fmt.Printf("  - %s: %d\n", l.Name, l.Port)
					}
				}
			}
			if hosts := egressHostList(gw.GetSpec().GetEgress().GetAllowlist().GetHosts()); len(hosts) > 0 {
				fmt.Println("Egress Allowlist:")
				for _, host := range hosts {
					fmt.Printf("  - %s\n", host)
				}
			}
		}
		return nil
	}

	task, err := client.GetTask(ctx, &v1alpha1.GetTaskRequest{Atespace: atespace, Name: name})
	if err != nil {
		return fmt.Errorf("getting task %q: %w", name, err)
	}

	taskName := ""
	taskAtespace := ""
	if task.Metadata != nil {
		taskName = task.Metadata.Name
		taskAtespace = task.Metadata.Atespace
	}
	phase := describeTaskPhase(task)
	actor := ""
	workerIP := ""
	var conditions []*v1alpha1.Condition
	if task.Status != nil {
		actor = task.Status.Actor
		workerIP = task.Status.WorkerIp
		conditions = task.Status.Conditions
	}

	fmt.Printf("Name:         %s\n", taskName)
	fmt.Printf("Atespace:     %s\n", taskAtespace)
	fmt.Printf("Phase:        %s\n", phase)
	fmt.Printf("Actor:        %s\n", displayActor(actor))
	fmt.Printf("Worker IP:    %s\n", displayWorkerIP(workerIP))
	if task.Spec != nil {
		if task.Spec.Gateway != nil {
			fmt.Printf("Gateway:      %s\n", task.Spec.Gateway.Name)
		}
		if refs := task.Spec.WorkspaceRefs(); len(refs) > 0 {
			paths := task.Spec.WorkspacePaths()
			fmt.Println("Workspaces:")
			for i, ref := range refs {
				fmt.Printf("  - %s  path=%s", ref.Name, paths[i])
				if ref.Goal != "" {
					fmt.Printf("  goal=%q", ref.Goal)
				}
				fmt.Println()
			}
		}
		if task.Spec.Image != "" {
			fmt.Printf("Image:        %s\n", task.Spec.Image)
		}
		if len(task.Spec.Command) > 0 {
			fmt.Printf("Command:      %s\n", formatCommandLine(task.Spec.Command))
		}
	}

	if len(conditions) > 0 {
		writeConditions(os.Stdout, conditions)
	}

	return nil
}

// writeConditions renders the Conditions section of `ax describe task`. The
// header has no leading blank line, matching every other describe section
// (Name/Workspaces/Image/Command print back-to-back); the stray blank line
// made Conditions look detached from the task it belongs to.
func writeConditions(w io.Writer, conditions []*v1alpha1.Condition) {
	if len(conditions) == 0 {
		return
	}
	fmt.Fprintln(w, "Conditions:")
	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "  TYPE\tSTATUS\tREASON\tMESSAGE")
	for _, c := range conditions {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", c.Type, c.Status, c.Reason, c.Message)
	}
	_ = tw.Flush()
}

func runWatch(serverURL, atespace string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ax watch task <name>")
	}
	kind, err := normalizeKind(args[0])
	if err != nil {
		return err
	}
	if kind != v1alpha1.KindTask {
		return fmt.Errorf("usage: ax watch task <name> (got kind %q)", args[0])
	}
	if len(args) > 2 {
		return fmt.Errorf("usage: ax watch task <name> (unexpected extra argument %q)", args[2])
	}
	name := args[1]

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	stream, err := client.WatchTask(context.Background(), &v1alpha1.WatchTaskRequest{Atespace: atespace, Name: name})
	if err != nil {
		return fmt.Errorf("watching task: %w", err)
	}

	// The banner is a diagnostic, not event data: it goes to stderr so the
	// event stream on stdout stays clean for piping, matching delete's
	// "waiting for ... to be deleted..." progress line.
	fmt.Fprintf(os.Stderr, "Watching task %s/%s...\n", atespace, name)

	return watchStreamLoop(stream, os.Stdout, atespace, name)
}

// isTerminalPhase reports whether the CLI watch loop should stop on phase.
// The set mirrors the server's isWatchTerminal (internal/server), which ends
// the stream on the same phases; the CLI keeps its own copy because it must
// also decide locally — e.g. against a server that keeps streaming past a
// terminal phase. "Terminating" was missing from this check, so such a
// stream would hang the CLI after printing the phase line.
func isTerminalPhase(phase string) bool {
	switch phase {
	case "Running", "Completed", "Failed", v1alpha1.PhaseTerminating:
		return true
	}
	return false
}

// watchStream is the Recv side of the WatchTask stream, as an interface so
// the drain loop is testable without a gRPC server.
type watchStream interface {
	Recv() (*v1alpha1.WatchTaskResponse, error)
}

// watchStreamLoop drains the watch stream until a terminal phase, stream end,
// or a fatal error. A NotFound from the server (unknown task name) surfaces
// as a clear message instead of a wrapped RPC error.
func watchStreamLoop(stream watchStream, w io.Writer, atespace, name string) error {
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if status.Code(err) == codes.NotFound {
				return fmt.Errorf("task %q not found in atespace %q", name, atespace)
			}
			return fmt.Errorf("receiving stream update: %w", err)
		}

		task := resp.Task
		if task != nil {
			phase := ""
			actor := ""
			workerIP := ""
			if task.Status != nil {
				phase = task.Status.Phase
				actor = task.Status.Actor
				workerIP = task.Status.WorkerIp
			}
			fmt.Fprintf(w, "[%s] Phase: %-10s Actor: %-18s WorkerIP: %s\n",
				time.Now().Format("15:04:05"),
				displayPhase(phase),
				displayActor(actor),
				displayWorkerIP(workerIP),
			)
			if isTerminalPhase(phase) {
				fmt.Fprintf(w, "Task reached terminal phase %q.\n", phase)
				return nil
			}
		}
	}
}

// runDelete removes one resource by kind and name.
func runDelete(serverURL, atespace string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ax delete <task|gateway|workspace|model> <name>")
	}
	if len(args) > 2 {
		return fmt.Errorf("usage: ax delete <task|gateway|workspace|model> <name> (unexpected extra argument %q)", args[2])
	}
	kind, err := normalizeKind(args[0])
	if err != nil {
		return err
	}

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Task deletion waits for the controller to tear down the actor, which can take
	// a while; the other kinds are removed synchronously.
	timeout := 15 * time.Second
	if kind == v1alpha1.KindTask {
		timeout = deleteTaskTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return deleteResource(ctx, client, kind, atespace, args[1])
}

const (
	deleteTaskTimeout  = 5 * time.Minute
	deletePollInterval = 500 * time.Millisecond
)

// deleteResource requests deletion, then blocks until the resource is really gone
// and prints a kubectl-style confirmation.
func deleteResource(ctx context.Context, client v1alpha1.AXClient, kind, atespace, name string) error {
	lower := strings.ToLower(kind)

	var err error
	switch kind {
	case v1alpha1.KindTask:
		_, err = client.DeleteTask(ctx, &v1alpha1.DeleteTaskRequest{Atespace: atespace, Name: name})
	case v1alpha1.KindGateway:
		_, err = client.DeleteGateway(ctx, &v1alpha1.DeleteGatewayRequest{Atespace: atespace, Name: name})
	case v1alpha1.KindWorkspace:
		_, err = client.DeleteWorkspace(ctx, &v1alpha1.DeleteWorkspaceRequest{Atespace: atespace, Name: name})
	case v1alpha1.KindModel:
		_, err = client.DeleteModel(ctx, &v1alpha1.DeleteModelRequest{Atespace: atespace, Name: name})
	default:
		return fmt.Errorf("unsupported kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("deleting %s %s/%s: %w", lower, atespace, name, err)
	}

	if err := waitForDeletion(ctx, client, kind, atespace, name); err != nil {
		return err
	}
	fmt.Printf("%s.ax.io/%s deleted\n", lower, name)
	return nil
}

// waitForDeletion polls until the resource returns NotFound or ctx expires.
func waitForDeletion(ctx context.Context, client v1alpha1.AXClient, kind, atespace, name string) error {
	lookup := func() error {
		var err error
		switch kind {
		case v1alpha1.KindTask:
			_, err = client.GetTask(ctx, &v1alpha1.GetTaskRequest{Atespace: atespace, Name: name})
		case v1alpha1.KindGateway:
			_, err = client.GetGateway(ctx, &v1alpha1.GetGatewayRequest{Atespace: atespace, Name: name})
		case v1alpha1.KindWorkspace:
			_, err = client.GetWorkspace(ctx, &v1alpha1.GetWorkspaceRequest{Atespace: atespace, Name: name})
		case v1alpha1.KindModel:
			_, err = client.GetModel(ctx, &v1alpha1.GetModelRequest{Atespace: atespace, Name: name})
		}
		return err
	}

	announced := false
	for {
		err := lookup()
		if status.Code(err) == codes.NotFound {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking %s %s/%s after delete: %w", strings.ToLower(kind), atespace, name, err)
		}
		if !announced {
			fmt.Fprintf(os.Stderr, "waiting for %s %s/%s to be deleted...\n", strings.ToLower(kind), atespace, name)
			announced = true
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s %s/%s to be deleted; it is still being torn down (check `ax describe` and the controller logs)", strings.ToLower(kind), atespace, name)
		case <-time.After(deletePollInterval):
		}
	}
}

// normalizeKind maps user-typed kinds ("task", "tasks", "Task") to the canonical
// manifest kind, rejecting anything unknown. Surrounding whitespace is
// trimmed: kinds are often pasted from docs or terminals with stray spaces,
// and " task" is unambiguously the task kind.
func normalizeKind(kind string) (string, error) {
	switch strings.TrimSuffix(strings.ToLower(strings.TrimSpace(kind)), "s") {
	case "task":
		return v1alpha1.KindTask, nil
	case "gateway":
		return v1alpha1.KindGateway, nil
	case "workspace":
		return v1alpha1.KindWorkspace, nil
	case "model":
		return v1alpha1.KindModel, nil
	case "":
		return "", errors.New("missing kind")
	default:
		return "", fmt.Errorf("unsupported kind %q (expected task, gateway, workspace, or model)", kind)
	}
}

// manifestFromArgs returns the manifest named by -f/--file (or stdin for "-").
// ok is false when no -f flag is present. The --file=<path> and -f=<path>
// forms are accepted like Go's flag package: the old code only matched the
// separate-token forms, so `ax apply --file=foo.yaml` fell through to
// "missing required flag: -f <file>" even though the user did pass --file.
func manifestFromArgs(args []string) (data []byte, ok bool, err error) {
	for i := 0; i < len(args); i++ {
		path, isFile := fileFlagValue(args[i])
		if !isFile {
			continue
		}
		if path == "" {
			// Bare "-f"/"--file": the path is the next arg. An attached
			// "--file=" with nothing after it is the same missing-value
			// error as a trailing bare "-f".
			if args[i] == "-f" || args[i] == "--file" {
				if i+1 >= len(args) {
					return nil, true, errors.New("-f requires a file path (or - for stdin)")
				}
				path = args[i+1]
			} else {
				return nil, true, errors.New("-f requires a file path (or - for stdin)")
			}
		}
		if path == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, true, fmt.Errorf("reading %s: %w", path, err)
		}
		return data, true, nil
	}
	return nil, false, nil
}

// fileFlagValue reports whether arg is apply's -f/--file flag and, for the
// =value forms, the attached path. A bare "-f"/"--file" returns isFile with
// an empty path: the caller reads the next arg.
func fileFlagValue(arg string) (path string, isFile bool) {
	if arg == "-f" || arg == "--file" {
		return "", true
	}
	if strings.HasPrefix(arg, "--file=") {
		return strings.TrimPrefix(arg, "--file="), true
	}
	if strings.HasPrefix(arg, "-f=") {
		return strings.TrimPrefix(arg, "-f="), true
	}
	return "", false
}

func runSuspend(serverURL, atespace string, args []string) error {
	name, err := parseSuspendResumeName("suspend", args)
	if err != nil {
		return err
	}

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := client.SuspendTask(ctx, &v1alpha1.SuspendTaskRequest{Atespace: atespace, Name: name}); err != nil {
		return fmt.Errorf("suspending task: %w", err)
	}

	fmt.Printf("task.ax.io/%s suspended\n", name)
	return nil
}

func runResume(serverURL, atespace string, args []string) error {
	name, err := parseSuspendResumeName("resume", args)
	if err != nil {
		return err
	}

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := client.ResumeTask(ctx, &v1alpha1.ResumeTaskRequest{Atespace: atespace, Name: name}); err != nil {
		return fmt.Errorf("resuming task: %w", err)
	}

	fmt.Printf("task.ax.io/%s resumed\n", name)
	return nil
}

// parseSuspendResumeName extracts the task name for `ax suspend`/`ax resume`:
// a bare name, or "task <name>" / "tasks <name>". The two-positional legacy
// form is kept. Anything else — no name or extra trailing args, which used to
// be silently ignored — is a usage error.
func parseSuspendResumeName(verb string, args []string) (string, error) {
	usage := fmt.Sprintf("usage: ax %s task <name>", verb)
	switch len(args) {
	case 1:
		return args[0], nil
	case 2:
		// The kind position is normalized like every other command (" task",
		// "Tasks", "TASK" all mean the task kind): the old exact match on
		// "task"/"tasks" treated `ax suspend " task" foo` as a bare name
		// " task" and silently dropped the real name.
		if kind, err := normalizeKind(args[0]); err == nil && kind == v1alpha1.KindTask {
			return args[1], nil
		}
		return args[0], nil
	default:
		return "", fmt.Errorf("%s", usage)
	}
}

func formatAge(d time.Duration) string {
	if d < 0 {
		// A CreationTimestamp in the future (client/server clock skew, a
		// restored backup) must not render as a negative age like "-5s".
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func runContext(kubeContext string, args []string) error {
	// ctx takes no positional args: anything after `ax ctx` is a typo
	// (`ax ctx frobnicate` printed the context and silently dropped
	// "frobnicate"). Same defect shape as get/describe/delete/watch/
	// tunnel/apply, so it gets the same "unexpected extra argument" usage
	// error instead of being silently ignored.
	if len(args) > 0 {
		return fmt.Errorf("usage: ax ctx (unexpected extra argument %q)", args[0])
	}
	ctxName, err := tunnel.CurrentContext(kubeContext)
	if err != nil || ctxName == "" {
		fmt.Println("No active Kubernetes context detected.")
		fmt.Println("Fallback server: http://localhost:8080 (or set $AX_SERVER / --server)")
		return nil
	}

	fmt.Printf("Active Kubernetes Context: %s\n", ctxName)
	if t, err := tunnel.GetTunnel(ctxName); err == nil && t != nil {
		status := "Healthy"
		if !tunnel.IsTunnelActive(t) {
			status = "Stale / Not responding"
		}
		fmt.Printf("AX Tunnel:                 http://127.0.0.1:%d -> %s/svc/%s:8080 (PID: %d, %s)\n",
			t.Port, t.Namespace, t.Service, t.PID, status)
	} else {
		fmt.Println("AX Tunnel:                 No background tunnel running (will auto-connect on next command)")
	}
	return nil
}

func runTunnel(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("specify tunnel action: 'ax tunnel list' or 'ax tunnel stop [context]'")
	}

	switch args[0] {
	case "list":
		if len(args) > 1 {
			return fmt.Errorf("usage: ax tunnel list")
		}
		tunnels, err := tunnel.ListTunnels()
		if err != nil {
			return err
		}
		if len(tunnels) == 0 {
			fmt.Println("No active tunnels found in ~/.ax/tunnels.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "CONTEXT\tLOCAL PORT\tREMOTE SERVICE\tPID\tSTATUS\tCREATED")
		for _, t := range tunnels {
			status := "Active"
			if !tunnel.IsTunnelActive(t) {
				status = "Stale"
			}
			fmt.Fprintf(w, "%s\t%d\t%s/%s\t%d\t%s\t%s\n",
				t.Context, t.Port, t.Namespace, t.Service, t.PID, status, t.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		return w.Flush()

	case "stop":
		if len(args) > 2 {
			return fmt.Errorf("usage: ax tunnel stop [context]")
		}
		if len(args) > 1 {
			ctxName := args[1]
			if err := tunnel.StopTunnelByContext(ctxName); err != nil {
				return fmt.Errorf("stopping tunnel for context %q: %w", ctxName, err)
			}
			fmt.Printf("Tunnel stopped for context %q\n", ctxName)
			return nil
		}
		cur, err := tunnel.CurrentContext("")
		if err != nil || cur == "" {
			return fmt.Errorf("no current context found to stop tunnel")
		}
		if err := tunnel.StopTunnelByContext(cur); err != nil {
			return fmt.Errorf("stopping tunnel for context %q: %w", cur, err)
		}
		fmt.Printf("Tunnel stopped for context %q\n", cur)
		return nil

	default:
		return fmt.Errorf("unknown tunnel subcommand: %s (available: list, stop)", args[0])
	}
}

// exitCodeForSignal maps an interrupt signal to the conventional shell exit
// code (128 + signal number).
func exitCodeForSignal(sig os.Signal) int {
	if sig == syscall.SIGTERM {
		return 128 + 15
	}
	return 128 + 2 // SIGINT and anything else interrupt-like
}

// watchSSHInterrupt installs a SIGINT/SIGTERM handler that releases the ssh
// session before the process dies. Go's default signal behavior kills the
// process without running deferred calls, so a Ctrl-C during `ax ssh` left
// the kubectl port-forward orphaned: its state entry kept claiming an active
// tunnel and the next command adopted a stale port-forward. The returned stop
// func unregisters the handler; exitFn is os.Exit in production and a stub in
// tests.
func watchSSHInterrupt(release func(), exitFn func(int)) (stop func()) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case sig := <-sigs:
			signal.Stop(sigs)
			if release != nil {
				release()
			}
			exitFn(exitCodeForSignal(sig))
		case <-done:
		}
	}()
	return func() {
		signal.Stop(sigs)
		close(done)
	}
}

// releaseSSHSession releases the resources held by an ssh session: the
// ephemeral atenet-router port-forward (nil on the direct-reachability path)
// and the gRPC connections. The non-zero remote exit path calls os.Exit,
// which never runs deferred calls, so this must be invoked explicitly before
// exiting — otherwise a failing remote command leaks a stray kubectl
// port-forward process.
func releaseSSHSession(cleanup func(), closers ...func() error) {
	if cleanup != nil {
		cleanup()
	}
	for _, c := range closers {
		if c != nil {
			_ = c()
		}
	}
}

// portForwardFn is tunnel.PortForward as a variable so tests can stub the
// kubectl port-forward.
var portForwardFn = tunnel.PortForward

// dialRouterGuest dials the guest daemon through the atenet-router
// port-forward and verifies the path speaks gRPC before returning. A dead
// router (port-forward up, nothing serving gRPC) fails fast here with a
// clear error instead of surfacing at the first Exec RPC.
func dialRouterGuest(kubeContext, targetActor string) (*guest.Client, func(), error) {
	localPort, pfCleanup, err := portForwardFn(context.Background(), kubeContext, "ate-system", "svc/atenet-router", 80)
	if err != nil {
		return nil, nil, fmt.Errorf("establishing port-forward to atenet-router: %w", err)
	}
	routerEndpoint := fmt.Sprintf("127.0.0.1:%d", localPort)
	c, err := guest.DialTarget(routerEndpoint, targetActor)
	if err != nil {
		pfCleanup()
		return nil, nil, fmt.Errorf("connecting to guest via atenet-router at %s: %w", routerEndpoint, err)
	}
	// The gRPC dial above is lazy: verify the router path actually speaks
	// gRPC before committing to it.
	readyCtx, readyCancel := context.WithTimeout(context.Background(), 2*time.Second)
	readyErr := c.WaitReady(readyCtx)
	readyCancel()
	if readyErr != nil {
		_ = c.Close()
		pfCleanup()
		return nil, nil, fmt.Errorf("atenet-router at %s not serving gRPC: %w", routerEndpoint, readyErr)
	}
	return c, pfCleanup, nil
}

// sshTargetActor builds the "atespace/actor" dial target for the guest
// session. Task metadata is nil-checked: the server may return a task
// without metadata, and dereferencing task.Metadata here panicked the CLI
// after a successful GetTask. An empty actor is an error: a Running task
// should always have one, and dialing "atespace/" would fail deep inside
// the router with an opaque error instead of naming the real problem. A
// whitespace-only actor is the same defect: dialing "atespace/ " fails just
// as opaquely, so it counts as missing too.
func sshTargetActor(task *v1alpha1.Task) (string, error) {
	if task == nil || task.Status == nil || strings.TrimSpace(task.Status.Actor) == "" {
		return "", fmt.Errorf("task %q has no actor assigned", sshTaskName(task))
	}
	atespace := ""
	if task.Metadata != nil {
		atespace = task.Metadata.Atespace
	}
	return fmt.Sprintf("%s/%s", atespace, strings.TrimSpace(task.Status.Actor)), nil
}

// sshTaskName is the nil-safe task name for ssh error messages.
func sshTaskName(task *v1alpha1.Task) string {
	if task != nil && task.Metadata != nil {
		return task.Metadata.Name
	}
	return ""
}

// sshTaskAndCommand splits the ssh args into the task name and the remote
// command. A leading "--" separator (kept in cleanArgs by parseGlobalArgs)
// is not a task name: `ax ssh -- mytask` must fetch "mytask", not try to
// fetch a task literally named "--".
func sshTaskAndCommand(args []string) (string, []string) {
	rest := args
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return "", nil
	}
	return rest[0], parseSSHCommand(rest[1:])
}

// parseSSHCommand extracts the remote command from the ssh args (everything
// after the task name). A "--" separator marks the rest of the line as the
// command verbatim; the words after it are appended to any command words
// already collected, so `ax ssh foo ls -- -a` runs `ls -a` instead of
// silently dropping `ls`.
func parseSSHCommand(args []string) []string {
	var cmdToRun []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			cmdToRun = append(cmdToRun, args[i+1:]...)
			break
		}
		cmdToRun = append(cmdToRun, args[i])
	}
	if len(cmdToRun) == 0 {
		cmdToRun = []string{"/bin/sh"}
	}
	return cmdToRun
}

func runSSH(serverURL, atespace, kubeContext string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ax ssh <task-name> [-- command...]")
	}

	taskName, cmdToRun := sshTaskAndCommand(args)
	if taskName == "" {
		return fmt.Errorf("usage: ax ssh <task-name> [-- command...]")
	}

	client, conn, err := getAXClient(serverURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	task, err := client.GetTask(ctx, &v1alpha1.GetTaskRequest{
		Atespace: atespace,
		Name:     taskName,
	})
	if err != nil {
		return fmt.Errorf("fetching task %q: %w", taskName, err)
	}

	if task == nil || task.Status == nil {
		return fmt.Errorf("task %q has no status available", taskName)
	}

	if task.Status.Phase != "Running" {
		return fmt.Errorf("task %q is in phase %q (must be Running to ssh)", taskName, task.Status.Phase)
	}

	if !task.GetSpec().GetDebug() {
		return fmt.Errorf("task %q does not expose guest services; set spec.debug: true and re-apply to enable ax ssh", taskName)
	}

	workerIP := strings.TrimSpace(task.Status.WorkerIp)
	if workerIP == "" {
		return fmt.Errorf("task %q has no worker IP assigned", taskName)
	}

	// The runner serves guest services on port 80 unless the worker IP says otherwise.
	host := workerIP
	port := 80
	if h, p, err := net.SplitHostPort(workerIP); err == nil {
		host = h
		if parsedPort, err := strconv.Atoi(p); err == nil {
			port = parsedPort
		}
	}

	targetActor, err := sshTargetActor(task)
	if err != nil {
		return err
	}
	var (
		guestClient *guest.Client
		cleanup     func()
	)

	// 1. First, check if worker IP is directly reachable (e.g. within cluster or local mesh)
	d := net.Dialer{Timeout: 500 * time.Millisecond}
	guestEndpoint := net.JoinHostPort(host, fmt.Sprint(port))
	directOK := false
	if testConn, dialErr := d.Dial("tcp", guestEndpoint); dialErr == nil {
		_ = testConn.Close()
		guestClient, err = guest.Dial(guestEndpoint)
		if err != nil {
			return fmt.Errorf("connecting to guest at %s: %w", guestEndpoint, err)
		}
		// The gRPC dial above is lazy: verify the direct path actually
		// speaks gRPC before committing to it. A half-open path (TCP
		// accepted, no gRPC server — stale worker IP, intercepting
		// middlebox) must fall back to the atenet-router instead of
		// failing at Exec with no fallback left.
		readyCtx, readyCancel := context.WithTimeout(context.Background(), 2*time.Second)
		readyErr := guestClient.WaitReady(readyCtx)
		readyCancel()
		if readyErr == nil {
			directOK = true
		} else {
			_ = guestClient.Close()
			guestClient = nil
		}
	}
	if !directOK {
		// 2. Connect via the Substrate atenet-router service in ate-system (port 80)
		var pfCleanup func()
		guestClient, pfCleanup, err = dialRouterGuest(kubeContext, targetActor)
		if err != nil {
			return err
		}
		cleanup = pfCleanup
	}

	if cleanup != nil {
		defer cleanup()
	}
	defer guestClient.Close()

	// A Ctrl-C here must not orphan the atenet-router port-forward: the
	// default signal death skips every deferred cleanup above. Release the
	// session explicitly on interrupt and exit with the conventional code.
	stopInterruptWatch := watchSSHInterrupt(func() {
		releaseSSHSession(cleanup, guestClient.Close, conn.Close)
	}, os.Exit)
	defer stopInterruptWatch()

	exitCode, err := guestClient.Exec(context.Background(), guest.ExecOptions{
		Command: cmdToRun,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	if err != nil {
		return err
	}

	if exitCode != 0 {
		// os.Exit skips the deferred cleanup/Close calls above: release the
		// port-forward and connections explicitly first.
		releaseSSHSession(cleanup, guestClient.Close, conn.Close)
		os.Exit(exitCode)
	}

	return nil
}

// listenerProtocol renders a gateway listener's protocol for display, or ""
// when the field is empty. Protocol is optional on the wire and nothing in
// this codebase defaults an empty value (there is no Gateway API defaulting
// here), so an empty protocol renders as no annotation rather than a
// fabricated "HTTP".
// egressHostLabel renders an egress allowlist entry for `ax describe
// gateway`. HostRule.Port is accepted by the manifest schema but nothing
// enforces it — ApplyEgressPolicy (internal/substrate) ignores it and
// GetPort() has no callers — so the label is the host alone. Rendering
// "host:port" would assert behavior the server doesn't implement (same
// fabrication class as the listener-protocol default). Matches the get
// EGRESS-HOSTS column, which already renders host only.
func egressHostLabel(h *v1alpha1.HostRule) string {
	if h == nil {
		return ""
	}
	return h.Host
}

// egressHostList renders the hosts of a gateway egress allowlist for CLI
// display: each host is trimmed and whitespace-only hosts are dropped (they
// rendered as blank entries in the list and describe views), matching the
// whitespace-as-empty rule the task actor/workerIP columns follow.
func egressHostList(hosts []*v1alpha1.HostRule) []string {
	var out []string
	for _, h := range hosts {
		if host := strings.TrimSpace(egressHostLabel(h)); host != "" {
			out = append(out, host)
		}
	}
	return out
}

// egressHostsLabel joins the display hosts for the get-list EGRESS-HOSTS
// column.
func egressHostsLabel(hosts []*v1alpha1.HostRule) string {
	return strings.Join(egressHostList(hosts), ",")
}

// describeTaskPhase is the Phase line for `ax describe task`. The get list
// defaults an unset phase to "Pending", and both stores set Phase to
// "Pending" on creation — describe does the same instead of printing a
// blank line for a status-less task.
func describeTaskPhase(task *v1alpha1.Task) string {
	// A whitespace-only phase (reachable: the stores default only "" to
	// "Pending") must read like the empty case, not render a blank field.
	// displayPhase also serves the get-list PHASE column, so the rule lives
	// in one place.
	if task != nil && task.Status != nil {
		return displayPhase(task.Status.Phase)
	}
	return "Pending"
}

// displayPhase renders a task phase for CLI display: whitespace-only counts
// as unset and reads "Pending"; otherwise the trimmed value renders. Used by
// `ax describe task` and the `ax get tasks` PHASE column.
func displayPhase(phase string) string {
	if p := strings.TrimSpace(phase); p != "" {
		return p
	}
	return "Pending"
}

// sanitizeCell renders a value for a tabwriter table cell: control
// characters (newline, tab, etc.) would split the row or shift the
// columns, so they render as spaces. Object names reach the server
// without control-character validation, so the list views must not
// trust them blindly.
func sanitizeCell(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// displayActor renders a task actor for CLI display: whitespace-only counts
// as unset and reads "<none>", matching the `ax get tasks` ACTOR column.
// Used by the get list, `ax describe task`, and the `ax watch` event lines
// so every render site follows one rule.
func displayActor(actor string) string {
	if a := strings.TrimSpace(actor); a != "" {
		return a
	}
	return "<none>"
}

// displayWorkerIP renders a task worker IP for CLI display: whitespace-only
// counts as unset and reads "<none>", matching the `ax get tasks` WORKER-IP
// column. Same render sites as displayActor.
func displayWorkerIP(workerIP string) string {
	if w := strings.TrimSpace(workerIP); w != "" {
		return w
	}
	return "<none>"
}

// displayBranch renders a workspace git repo branch for `ax describe
// workspace`: whitespace-only counts as unset and reads "main", matching the
// runner's workspace setup, which defaults unset branches to main before
// fetching. Without this, `ax describe workspace` showed the raw whitespace
// while the runner had to fail its git fetch on a " " branch first.
func displayBranch(branch string) string {
	if b := strings.TrimSpace(branch); b != "" {
		return b
	}
	return "main"
}

func listenerProtocol(l *v1alpha1.Listener) string {
	// A padded protocol (" http ") renders with stray spaces in the
	// get-list "80/ http " and describe "80 ( http )" views; displayPhase
	// trims padded phases, so the protocol rule trims too.
	if l == nil {
		return ""
	}
	return strings.TrimSpace(l.Protocol)
}

// formatParamValue renders a model parameter value for `ax describe model`.
// Printing the raw value with %v leaks Go syntax ("map[x:y]", "[p q]") for
// nested maps and slices; this renders them recursively in a readable
// key=value / comma-joined form instead. Scalar values keep their %v form.
func formatParamValue(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+formatParamValue(t[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, formatParamValue(e))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// sortByName orders list rows by their object name so `ax get` output is
// deterministic. The server returns items in store order, which can reshuffle
// across restarts; kubectl sorts list output by name too.
func sortByName[T any](items []T, name func(T) string) []T {
	out := make([]T, len(items))
	copy(out, items)
	sort.SliceStable(out, func(i, j int) bool { return name(out[i]) < name(out[j]) })
	return out
}

func objectName(meta *v1alpha1.ObjectMeta) string {
	if meta != nil {
		return meta.Name
	}
	return ""
}
