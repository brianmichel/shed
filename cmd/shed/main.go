package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/brianmichel/shed/internal/agent"
	"github.com/brianmichel/shed/internal/cli"
	"github.com/brianmichel/shed/internal/client"
	"github.com/brianmichel/shed/internal/compute"
	shedconfig "github.com/brianmichel/shed/internal/config"
	"github.com/brianmichel/shed/internal/dev"
	"github.com/brianmichel/shed/internal/job"
	"github.com/brianmichel/shed/internal/server"
	"github.com/brianmichel/shed/internal/store"
	"github.com/spf13/cobra"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Cobra prints its own "Error: ..." plus contextual usage on failure, so
	// there is nothing left for main to print here — just set the exit code.
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "shed",
		Short: "Shed is a single-binary control plane and execution runtime for sandboxes.",
	}
	root.AddCommand(newServerCmd(), newClientCmd(), newDevCmd(), cli.NewJobCommand())
	return root
}

// runDaemon runs a long-lived server/client/dev process. Unlike `shed job`,
// daemon failures are fatal crashes worth a timestamp, so they bypass
// cobra's error return entirely and log.Fatal directly.
func runDaemon(fn func() error) error {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if err := fn(); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
	return nil
}

func newServerCmd() *cobra.Command {
	var (
		addr           string
		uiEnabled      bool
		defaultCompute string
		workspace      string
		plugins        pluginConfigFlag
		configPath     string
	)
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the control-plane server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return runDaemon(func() error {
				fileCfg, err := shedconfig.Load(configPath)
				if err != nil {
					return err
				}
				if defaultCompute == "local" && fileCfg.Compute.DefaultDriver != "" {
					defaultCompute = fileCfg.Compute.DefaultDriver
				}
				externals, err := plugins.externalConfigs()
				if err != nil {
					return err
				}
				externals = append(externals, fileCfg.ExternalComputes()...)
				mgr, err := buildComputeManagerFromConfigs(ctx, defaultCompute, workspace, externals, fileCfg.ComputeClasses)
				if err != nil {
					return err
				}
				st := store.NewMemoryStore()
				srv := server.New(server.Config{Addr: addr, UIEnabled: uiEnabled, ComputeManager: mgr, DefaultCompute: defaultCompute}, st)
				agentMgr := agent.NewManager(agent.Config{Store: st, Commands: srv})
				srv.SetJobManager(job.NewManager(job.Config{Store: st, Sandboxes: srv, Agent: agentMgr}))
				return srv.Start(ctx)
			})
		},
	}
	fs := cmd.Flags()
	fs.StringVar(&addr, "addr", envOr("SHED_ADDR", "127.0.0.1:6464"), "HTTP listen address")
	fs.BoolVar(&uiEnabled, "ui", true, "serve embedded operator UI")
	fs.StringVar(&defaultCompute, "compute-driver", envOr("SHED_COMPUTE_DRIVER", "local"), "default sandbox compute driver")
	fs.StringVar(&workspace, "compute-workspace-root", envOr("SHED_COMPUTE_WORKSPACE", ".shed-server/workspace"), "workspace root for the built-in local compute")
	plugins.setMany(envOr("SHED_COMPUTE_PLUGINS", ""))
	fs.Var(&plugins, "compute-plugin", "external compute plugin as name=/path/to/plugin; repeatable")
	fs.StringVar(&configPath, "config", envOr("SHED_CONFIG", ""), "JSON config file path")
	return cmd
}

func newClientCmd() *cobra.Command {
	var (
		serverURL, sessionKey, sessionID, sandboxID, workspace string
	)
	cmd := &cobra.Command{
		Use:   "client",
		Short: "Run the in-compute client (executes inside a sandbox)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(func() error {
				c, err := client.New(client.Config{ServerURL: serverURL, SessionKey: sessionKey, SessionID: sessionID, SandboxID: sandboxID, WorkspaceRoot: workspace, HeartbeatEvery: 10 * time.Second})
				if err != nil {
					return err
				}
				return c.Run(cmd.Context())
			})
		},
	}
	fs := cmd.Flags()
	fs.StringVar(&serverURL, "server", envOr("SHED_SERVER_URL", "ws://127.0.0.1:6464/v1/client/connect"), "server websocket URL")
	fs.StringVar(&sessionKey, "session-key", envOr("SHED_SESSION_KEY", ""), "client session key")
	fs.StringVar(&sessionID, "session-id", envOr("SHED_SESSION_ID", ""), "client session id")
	fs.StringVar(&sandboxID, "sandbox-id", envOr("SHED_SANDBOX_ID", ""), "sandbox id")
	fs.StringVar(&workspace, "workspace-root", envOr("SHED_WORKSPACE_ROOT", "/tmp"), "workspace root")
	fs.String("config", "", "config file path (reserved)")
	return cmd
}

func newDevCmd() *cobra.Command {
	var (
		addr           string
		workspace      string
		uiEnabled      bool
		defaultCompute string
		plugins        pluginConfigFlag
		configPath     string
	)
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Run server and client together for local development",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return runDaemon(func() error {
				fileCfg, err := shedconfig.Load(configPath)
				if err != nil {
					return err
				}
				if defaultCompute == "local" && fileCfg.Compute.DefaultDriver != "" {
					defaultCompute = fileCfg.Compute.DefaultDriver
				}
				externals, err := plugins.externalConfigs()
				if err != nil {
					return err
				}
				externals = append(externals, fileCfg.ExternalComputes()...)
				return dev.Run(ctx, dev.Config{Addr: addr, WorkspaceRoot: workspace, UIEnabled: uiEnabled, DefaultCompute: defaultCompute, ExternalComputes: externals, ComputeClasses: fileCfg.ComputeClasses})
			})
		},
	}
	fs := cmd.Flags()
	fs.StringVar(&addr, "addr", envOr("SHED_DEV_ADDR", "127.0.0.1:6464"), "HTTP listen address")
	fs.StringVar(&workspace, "workspace-root", envOr("SHED_DEV_WORKSPACE", ".shed-dev/workspace"), "workspace root")
	fs.BoolVar(&uiEnabled, "ui", true, "serve embedded operator UI")
	fs.StringVar(&defaultCompute, "compute-driver", envOr("SHED_DEV_COMPUTE_DRIVER", "local"), "default sandbox compute driver")
	plugins.setMany(envOr("SHED_DEV_COMPUTE_PLUGINS", ""))
	fs.Var(&plugins, "compute-plugin", "external compute plugin as name=/path/to/plugin; repeatable")
	fs.StringVar(&configPath, "config", envOr("SHED_DEV_CONFIG", envOr("SHED_CONFIG", "")), "JSON config file path")
	return cmd
}

// pluginConfigFlag is a repeatable name=/path/to/plugin flag, implementing
// both stdlib flag.Value and pflag.Value (they share the same Set/String
// shape; pflag additionally requires Type).
type pluginConfigFlag []string

func (f *pluginConfigFlag) String() string { return strings.Join(*f, ",") }
func (f *pluginConfigFlag) Set(v string) error {
	if strings.TrimSpace(v) != "" {
		*f = append(*f, strings.TrimSpace(v))
	}
	return nil
}
func (f *pluginConfigFlag) Type() string { return "name=path" }
func (f *pluginConfigFlag) setMany(v string) {
	for _, part := range strings.Split(v, ",") {
		_ = f.Set(part)
	}
}
func (f pluginConfigFlag) externalConfigs() ([]compute.ExternalPluginConfig, error) {
	out := make([]compute.ExternalPluginConfig, 0, len(f))
	for _, spec := range f {
		name, command, ok := strings.Cut(spec, "=")
		if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(command) == "" {
			return nil, fmt.Errorf("compute plugin must be name=/path/to/plugin: %q", spec)
		}
		out = append(out, compute.ExternalPluginConfig{Name: strings.TrimSpace(name), Command: strings.TrimSpace(command)})
	}
	return out, nil
}

func buildComputeManagerFromConfigs(ctx context.Context, defaultCompute, workspace string, externals []compute.ExternalPluginConfig, classes []compute.SandboxClass) (*compute.Manager, error) {
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: defaultCompute})
	_ = mgr.RegisterBuiltin("local", compute.NewLocalCompute(ctx, compute.LocalConfig{WorkspaceRoot: workspace, HeartbeatEvery: 5 * time.Second}))
	for _, ext := range externals {
		if err := mgr.RegisterExternal(ext); err != nil {
			return nil, err
		}
	}
	registerDefaultLocalClasses(mgr)
	for _, class := range classes {
		if err := mgr.RegisterClass(class); err != nil {
			return nil, err
		}
	}
	return mgr, nil
}

func registerDefaultLocalClasses(mgr *compute.Manager) {
	_ = mgr.RegisterClass(compute.SandboxClass{Name: "local", Driver: "local", Description: "Local host workspace sandbox", Capabilities: map[string]any{"exec": true, "files": true}})
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
