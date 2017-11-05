package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gigawattio/errorlib"
	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks"
	"github.com/jaytaylor/shipbuilder/pkg/core"
	"github.com/jaytaylor/shipbuilder/pkg/domain"
	"github.com/jaytaylor/shipbuilder/pkg/releases"
	"github.com/jaytaylor/shipbuilder/pkg/version"

	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v2"
)

var (
	appFlag = &cli.StringFlag{
		Name:    "app",
		Aliases: []string{"a", "app-name"},
		Usage:   "Name of app",
	}
)

func main() {
	app := &cli.App{
		Name:        "shipbuilder",
		Version:     version.Version,
		Description: "Welcome to Shipbuilder!",
		Usage:       "Shipbuilder client",
		Action: func(ctx *cli.Context) error {
			client := &core.Client{}
			client.Do(os.Args) // ctx.Args().Slice())
			return nil
		},
		Commands: []*cli.Command{
			&cli.Command{
				Name:    "client",
				Aliases: []string{"c"},
				Action: func(ctx *cli.Context) error {
					client := &core.Client{}
					client.Do(ctx.Args().Slice())
					return nil
				},
			},
			&cli.Command{
				Name:    "server",
				Aliases: []string{"s"},
				Flags: []cli.Flag{
					&cli.BoolFlag{ // TODO: Change to bool.
						Name:        "haproxy-stats",
						EnvVars:     []string{"SB_HAPROXY_STATS"},
						Usage:       "Control whether or not generated HAProxy configs will have statistics enabled",
						Value:       core.DefaultHAProxyStats,
						Destination: &core.DefaultHAProxyStats,
					},
					&cli.StringFlag{
						Name:        "haproxy-credentials",
						EnvVars:     []string{"SB_HAPROXY_CREDENTIALS"},
						Usage:       "HAProxy user:secret",
						Value:       core.DefaultHAProxyCredentials,
						Destination: &core.DefaultHAProxyCredentials,
					},
					&cli.StringFlag{
						Name:        "aws-key",
						EnvVars:     []string{"SB_AWS_KEY"},
						Usage:       "AWS key",
						Value:       core.DefaultAWSKey,
						Destination: &core.DefaultAWSKey,
					},
					&cli.StringFlag{
						Name:        "aws-secret",
						EnvVars:     []string{"SB_AWS_SECRET"},
						Usage:       "AWS secret",
						Value:       core.DefaultAWSSecret,
						Destination: &core.DefaultAWSSecret,
					},
					&cli.StringFlag{
						Name:        "aws-region",
						EnvVars:     []string{"SB_AWS_REGION"},
						Usage:       "AWS region to use",
						Value:       core.DefaultAWSRegion,
						Destination: &core.DefaultAWSRegion,
					},
					&cli.StringFlag{
						Name:        "s3-bucket",
						EnvVars:     []string{"SB_S3_BUCKET"},
						Usage:       "Name of S3 bucket where app releases will be stored",
						Value:       core.DefaultS3BucketName,
						Destination: &core.DefaultS3BucketName,
					},
					&cli.StringFlag{
						Name:        "ssh-host",
						EnvVars:     []string{"SB_SSH_HOST"},
						Usage:       "Address of the server host for the client to connect to",
						Value:       core.DefaultSSHHost,
						Destination: &core.DefaultSSHHost,
					},
					&cli.StringFlag{
						Name:        "ssh-key",
						EnvVars:     []string{"SB_SSH_KEY"},
						Usage:       "Location of SSH key for the client to use",
						Value:       core.DefaultSSHKey,
						Destination: &core.DefaultSSHKey,
					},
					&cli.StringFlag{
						Name:        "lxc-fs",
						EnvVars:     []string{"SB_LXC_FS"},
						Usage:       "LXC filesystem type",
						Value:       core.DefaultLXCFS,
						Destination: &core.DefaultLXCFS,
					},
					&cli.StringFlag{
						Name:        "zfs-pool",
						EnvVars:     []string{"SB_ZFS_POOL"},
						Usage:       "ZFS pool name",
						Value:       core.DefaultZFSPool,
						Destination: &core.DefaultZFSPool,
					},
					&cli.StringFlag{
						Name:    "releases-provider",
						EnvVars: []string{"SB_RELEASES_PROVIDER"},
						Usage:   "Release persistence backend, must be one of: 'aws', 'fs'",
						Value:   "aws",
					},
					&cli.StringFlag{
						Name:    "fs-releases-provider-path",
						Aliases: []string{"fs-path"},
						EnvVars: []string{"SB_FS_RELEASES_PROVIDER_PATH"},
						Usage:   "Storage path for FS releases provider",
					},
				},
				Before: func(ctx *cli.Context) error {
					if ctx.Args().Len() == 0 {
						if err := core.ValidateConfig(); err != nil {
							return err
						}
					}
					return nil
				},
				Action: func(ctx *cli.Context) error {
					releasesProvider, err := releasesProvider(ctx)
					if err != nil {
						return err
					}

					server := &core.Server{
						BuildpacksProvider: bindata_buildpacks.NewProvider(),
						ReleasesProvider:   releasesProvider,
					}
					if err := server.Start(); err != nil {
						return err
					}
					if err := sigWait(); err != nil {
						return err
					}
					return nil
				},
				Subcommands: []*cli.Command{
					&cli.Command{
						Name:        "showconfig",
						Aliases:     []string{"show-config"},
						Description: "Print current configuration",
						Action: func(ctx *cli.Context) error {
							type pair struct {
								key   string
								value interface{}
							}
							pairs := []pair{
								{"DefaultHAProxyStats", core.DefaultHAProxyStats},
								{"DefaultHAProxyCredentials", core.DefaultHAProxyCredentials},
								{"DefaultAWSKey", core.DefaultAWSKey},
								{"DefaultAWSSecret", core.DefaultAWSSecret},
								{"DefaultAWSRegion", core.DefaultAWSRegion},
								{"DefaultS3BucketName", core.DefaultS3BucketName},
								{"DefaultSSHHost", core.DefaultSSHHost},
								{"DefaultSSHKey", core.DefaultSSHKey},
								{"DefaultLXCFS", core.DefaultLXCFS},
								{"DefaultZFSPool", core.DefaultZFSPool},
							}
							for _, p := range pairs {
								fmt.Fprintf(os.Stdout, "%v: %v\n", p.key, p.value)
							}
							return nil
						},
					},
				},
			},

			////////////////////////////////////////////////////////////////////
			// Client commands

			&cli.Command{
				Name:        "buildpacks",
				Aliases:     []string{"build-packs"},
				Description: "List available build-packs",
				Action: func(ctx *cli.Context) error {
					for _, name := range bindata_buildpacks.NewProvider().Available() {
						fmt.Fprintf(os.Stdout, "%v\n", name)
					}
					return nil
				},
				Subcommands: buildpackSubcommands(),
			},

			// &cli.Command{
			// 	Name:        "buildpacks",
			// 	Aliases:     []string{"build-packs"},
			// 	Description: "List available build-packs",
			// 	Action: func(ctx *cli.Context) error {
			// 		for _, name := range bindata_buildpacks.NewProvider().Available() {
			// 			fmt.Fprintf(os.Stdout, "%v\n", name)
			// 		}
			// 		return nil
			// 	},
			// },

			////////////////////////////////////////////////////////////////////
			// App management commands                                        //
			////////////////////////////////////////////////////////////////////

			////////////////////////////////////////////////////////////////////
			// apps:*
			command(
				[]string{"apps", "apps:list", "Apps_List"},
				"List shipbuilder-managed apps",
			),
			appCommand(
				[]string{"create", "apps:create", "Apps_Create"},
				"Create a new app",
				flagSpec{
					names:    []string{"buildpack", "b"},
					usage:    "Desired buildpack for app type",
					required: true,
				},
			),
			appCommand(
				[]string{"destroy", "apps:destroy", "delete", "Apps_Destroy"},
				"Destroy an app",
			),
			command(
				[]string{"clone", "apps:clone", "Apps_Clone"},
				"Clone an app",
				flagSpec{
					names:    []string{"old-app", "o"},
					usage:    "Name of old app",
					required: true,
				},
				flagSpec{
					names:    []string{"new-app", "n"},
					usage:    "Name of new app",
					required: true,
				},
			),

			// HERE

			////////////////////////////////////////////////////////////////////
			// config:*
			appCommand(
				[]string{"config", "config:list", "Config_List"},
				"Show the configuration for an app",
			),
			&cli.Command{
				Name:        "config:get",
				Aliases:     []string{"config:show", "Config_Get"},
				Description: "Get the value of a configuration parameter for an app",
				Flags: []cli.Flag{
					appFlag,
					&cli.StringFlag{
						Name:    "key",
						Aliases: []string{"k", "parameter", "p"},
						Usage:   "Configuration parameter to lookup",
					},
				},
				Action: func(ctx *cli.Context) error {
					var (
						app = ctx.String("app")
						key = ctx.String("key")
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(key) == 0 {
						return errors.New("key flag is required")
					}
					return (&core.Client{}).RemoteExec("Config_Get", app, key)
				},
			},
			deferredMappedAppCommand(
				[]string{"set", "config:set", "Config_Set"},
				"Set the value of one or more configuration parameters for an app in the form of FOO=bar BAZ=xy",
			),
			&cli.Command{
				Name:        "config:remove",
				Aliases:     []string{"config:rm", "Config_Remove"},
				Description: "Remove one or more configuration keys from an app",
				Flags: []cli.Flag{
					appFlag,
					&cli.BoolFlag{
						Name:    "deferred",
						Aliases: []string{"defer", "d"},
						Usage:   "Defer app redeployment",
					},
					&cli.StringSliceFlag{
						Name:    "key",
						Aliases: []string{"k", "parameter", "p"},
						Usage:   "Pass multiple time for multiple keys",
					},
				},
				Action: func(ctx *cli.Context) error {
					var (
						app      = ctx.String("app")
						deferred = ctx.Bool("deferred")
						keys     = ctx.StringSlice("key")
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(keys) == 0 {
						keys = ctx.Args().Slice()
						if len(keys) == 0 {
							return errors.New("key flag(s) or args are required")
						}
					}
					return (&core.Client{}).RemoteExec("Config_Remove", app, deferred, keys)
				},
			},

			////////////////////////////////////////////////////////////////////
			// domains:*
			appCommand(
				[]string{"domains", "domains:list", "Domains_List"},
				"Show domain names associated with an app",
			),
			&cli.Command{
				Name:        "domains:add",
				Aliases:     []string{"domain:add", "Domains_Add"},
				Description: "Associate one or more domain names to an app",
				Flags: []cli.Flag{
					appFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app     = ctx.String("app")
						domains = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(domains) == 0 {
						return errors.New("cannot add empty list of domains to app")
					}
					return (&core.Client{}).RemoteExec("Domains_Add", app, domains)
				},
			},
			&cli.Command{
				Name:        "domains:remove",
				Aliases:     []string{"domain:remove", "Domains_Remove"},
				Description: "Remove one or more domain names from an app",
				Flags: []cli.Flag{
					appFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app     = ctx.String("app")
						domains = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(domains) == 0 {
						return errors.New("cannot remove empty list of domains from app")
					}
					return (&core.Client{}).RemoteExec("Domains_Remove", app, domains)
				},
			},

			////////////////////////////////////////////////////////////////////
			// drains:*
			appCommand(
				[]string{"drains", "drains:list", "Drains_List"},
				"Show drains for an app",
			),
			&cli.Command{
				Name:        "drains:add",
				Aliases:     []string{"drain:add", "Drains_Add"},
				Description: "Add one or more drains to an app",
				Flags: []cli.Flag{
					appFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app    = ctx.String("app")
						drains = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(drains) == 0 {
						return errors.New("cannot add empty list of drains to app")
					}
					return (&core.Client{}).RemoteExec("Drains_Add", app, drains)
				},
			},
			&cli.Command{
				Name:        "drains:remove",
				Aliases:     []string{"drain:remove", "Drains_Remove"},
				Description: "Remove one or more drains from an app",
				Flags: []cli.Flag{
					appFlag,
				},
				Action: func(ctx *cli.Context) error {
					var (
						app    = ctx.String("app")
						drains = ctx.Args().Slice()
					)
					if len(app) == 0 {
						return errors.New("app flag is required")
					}
					if len(drains) == 0 {
						return errors.New("cannot remove empty list of drains from app")
					}
					return (&core.Client{}).RemoteExec("Drains_Remove", app, drains)
				},
			},

			////////////////////////////////////////////////////////////////////
			// redeploy
			appCommand(
				[]string{"redeploy", "apps:redeploy", "Redeploy_App"},
				"Redeploy the current running version of an app",
			),
			// TODO: Verify that this alwaays redeploys the current version and not the latest version.

			////////////////////////////////////////////////////////////////////
			// deploy
			appCommand(
				[]string{"deploy", "Deploy"},
				"Deploy an app at a specific version",
				flagSpec{
					names:    []string{"version", "v", "revision", "r"},
					usage:    "Version to use",
					required: true,
				},
			),

			////////////////////////////////////////////////////////////////////
			// reset
			appCommand(
				[]string{"reset", "apps:reset", "Reset_App"},
				"Reset all build artifacts for an app so the next deployment will build from scratch",
			),

			////////////////////////////////////////////////////////////////////
			// logs:*
			appCommand(
				[]string{"logs", "logs:get", "Logs_Get"},
				"Get logs for an app",
				flagSpec{
					names: []string{"process", "p"},
					usage: "App process name",
				},
				flagSpec{
					names: []string{"filter"},
					usage: "Golang regular exression to filter log lines on",
				},
			),

			////////////////////////////////////////////////////////////////////
			// logger
			appCommand(
				[]string{"logger", "Logger"},
				"Logger command ??? TODO: test this!",
				flagSpec{
					names:    []string{"host", "h"},
					usage:    "Slae node name (e.g. hostname)",
					required: true,
				},
				flagSpec{
					names:    []string{"process", "p"},
					usage:    "App process name",
					required: true,
				},
			),

			////////////////////////////////////////////////////////////////////
			// run
			appCommand(
				[]string{"run", "shell", "console", "Console"},
				"Run a command in an app container image",
				flagSpec{
					names:    []string{"command", "c"},
					usage:    "Command to use",
					required: true,
				},
			),
			// TODO: consider adding command to attach to a running container.

			////////////////////////////////////////////////////////////////////
			// maint:*
			appCommand(
				[]string{"maintenance:url", "maint:url", "Maintenance_Url"},
				"Set the maintenance redirect URL for an app",
				flagSpec{
					names:    []string{"url", "u"},
					usage:    "Maintenance URL",
					required: true,
				},
			),
			appCommand(
				[]string{"maintenance", "maintenance:status", "maint", "maint:status", "Maintenance_Status"},
				"Show maintenance mode status for an app",
			),
			appCommand(
				[]string{"maintenance:on", "maint:on", "Maintenance_On"},
				"Activates maintenance mode for an app",
			),
			appCommand(
				[]string{"maintenance:off", "maint:off", "Maintenance_Off"},
				"Deactivates maintenance mode for an app",
			),

			////////////////////////////////////////////////////////////////////
			// privatekey:*
			appCommand(
				[]string{"privatekey", "privatekey:get", "PrivateKey_Get"},
				"Show SSH private key to use for accessing and cloning protected repositories when checking out git submodules for app",
			),
			appCommand(
				[]string{"privatekey:set", "PrivateKey_Set"},
				"Set the maintenance redirect URL for an app",
				flagSpec{
					names:    []string{"private-key"},
					usage:    "Private SSH keystring",
					required: true,
				},
			),
			appCommand(
				[]string{"privatekey:remove", "privatekey:rm", "privatekey:delete", "PrivateKey_Remove"},
				"Remove existing SSH private key from app",
			),

			////////////////////////////////////////////////////////////////////
			// ps:*
			appCommand(
				[]string{"ps", "ps:list", "Ps_List"},
				"Show running container processes for app",
			),
			deferredMappedAppCommand(
				[]string{"ps:scale", "scale", "Ps_Scale"},
				"Scale app processes up or down",
			),
			argsOrFlagAppCommand(
				[]string{"ps:status", "status", "Ps_Status"},
				"Get the status of one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),
			argsOrFlagAppCommand(
				[]string{"ps:restart", "restart", "Ps_Restart"},
				"Restart one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),
			argsOrFlagAppCommand(
				[]string{"ps:stop", "stop", "Ps_Stop"},
				"Stop one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),
			argsOrFlagAppCommand(
				[]string{"ps:start", "start", "Ps_Start"},
				"Start one or more container processes for an app",
				[]string{"process-types"},
				"Specify flag multiple times for multiple process types",
			),

			////////////////////////////////////////////////////////////////////
			// rollback
			appCommand(
				[]string{"rollback", "Rollback"},
				"Roll app back to previous release",
				flagSpec{
					names: []string{"version", "v"},
					usage: "Version to rollback to - if omitted, then the previous version will be used",
				},
			),

			////////////////////////////////////////////////////////////////////
			// releases:*
			appCommand(
				[]string{"releases", "releases:list", "Releases_List"},
				"Show app release history",
			),
			appCommand(
				[]string{"releases:info", "release:info", "Releases_Info"},
				"Show detailed release history information for a specific version",
				flagSpec{
					names: []string{"version", "v"},
					usage: "Version to rollback to - if omitted, then the previous version will be used",
				},
			),

			////////////////////////////////////////////////////////////////////
			// Global system management commands                              //
			////////////////////////////////////////////////////////////////////

			command(
				[]string{"health", "apps:health", "App_Health"},
				"Show health report for all apps",
			),
			////////////////////////////////////////////////////////////////////
			// lb:*
			command(
				[]string{"lb", "lb:list", "LoadBalancer_List"},
				"Show server load-balancers",
			),
			argsOrFlagAppCommand(
				[]string{"lb:add", "LoadBalancer_Add"},
				"Add one or more load-balancers to the server",
				[]string{"hostname"},
				"Specify flag multiple times for multiple load-balancer hostnames",
			),
			argsOrFlagAppCommand(
				[]string{"lb:remove", "LoadBalancer_Remove"},
				"Remove one or more load-balancers from the server",
				[]string{"hostname"},
				"Specify flag multiple times for multiple load-balancer hostnames",
			),

			////////////////////////////////////////////////////////////////////
			// nodes:*
			command(
				[]string{"nodes", "nodes:list", "slaves:list", "Nodes_List"},
				"Show server slave nodes",
			),
			argsOrFlagAppCommand(
				[]string{"nodes:add", "slaves:add", "slave:add", "Nodes_Add"},
				"Add one or more slave nodes to the server",
				[]string{"hostname"},
				"Specify flag multiple times for multiple slave node hostnames",
			),
			argsOrFlagAppCommand(
				[]string{"nodes:remove", "slaves:remove", "slave:remove", "Nodes_Remove"},
				"Remove one or more slave nodes from the server",
				[]string{"hostname"},
				"Specify flag multiple times for multiple slave node hostnames",
			),

			////////////////////////////////////////////////////////////////////
			// runtime:*
			// DISABLED:
			// global("runtime:tests", "runtimetests", "LocalRuntimeTests"),

			////////////////////////////////////////////////////////////////////
			// sys:*
			command(
				[]string{"system:zfscleanup", "sys:zfscleanup", "System_ZfsCleanup"},
				"Cleans up old app build versions on the shipbuilder build box; IMPORTANT: this is automated via cron, so it shoud not need to be run manually",
			),
			command(
				[]string{"system:snapshotscleanup", "sys:snapshotscleanup", "System_SnapshotsCleanup"},
				"Prune orphaned ZFS snapshots; IMPORTANT: this is automated via cron, so it shoud not need to be run manually",
			),
			command(
				[]string{"system:ntpsync", "sys:ntpsync", "System_NtpSync"},
				"Sync system clock via NTP; IMPORTANT: this is automated via cron, so it shoud not need to be run manually",
			),

			////////////////////////////////////////////////////////////////////
			// Backend functions                                              //
			////////////////////////////////////////////////////////////////////

			////////////////////////////////////////////////////////////////////
			// pre/post-receive hooks
			command(
				[]string{"pre-receive", "PreReceive"},
				"Shipbuilder server git pre-receive hook function",
				flagSpec{
					names:    []string{"directory", "d"},
					usage:    "Path to directory",
					required: true,
				},
				flagSpec{
					names:    []string{"oldrev", "o"},
					usage:    "old git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"newrev", "n"},
					usage:    "new git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"ref", "r"},
					usage:    "git ref",
					required: true,
				},
			),
			command(
				[]string{"post-receive", "PostReceive"},
				"Shipbuilder server git post-receive hook function",
				flagSpec{
					names:    []string{"directory", "d"},
					usage:    "Path to directory",
					required: true,
				},
				flagSpec{
					names:    []string{"oldrev", "o"},
					usage:    "old git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"newrev", "n"},
					usage:    "new git revision",
					required: true,
				},
				flagSpec{
					names:    []string{"ref", "r"},
					usage:    "git ref",
					required: true,
				},
			),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Errorf("%s", err)
		os.Exit(1)
	}
}

func sigWait() error {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh

	return nil
}

func releasesProvider(ctx *cli.Context) (provider domain.ReleasesProvider, err error) {
	requested := ctx.String("releases-provider")

	switch requested {
	case "aws":
		provider = releases.NewAWSS3ReleasesProvider(core.DefaultAWSKey, core.DefaultAWSSecret, core.DefaultS3BucketName, core.DefaultAWSRegion)
		return

	case "fs":
		storagePath := ctx.String("fs-releases-provider-path")
		if len(storagePath) == 0 {
			err = errors.New("missing required parameter: fs-releases-provider-path")
			return
		}
		provider = releases.NewFSReleasesProvider(storagePath)
		return
	}

	err = fmt.Errorf("unrecognized releases-provider %q", requested)
	return
}

type flagSpec struct {
	names    []string // NB: pos[0] = name, pos[1:] = aliases.
	usage    string
	required bool
}

// command generates a cli.Command with 0 or more string flags.
//
// In the generated function, if args count matches required flags count and
// flag values are empty, positional args will used.
//
// Important: names must be a non-empty slice and end with a value which
// corresponds to a valid shipbuilder server method.
func command(names []string, description string, flagSpecs ...flagSpec) *cli.Command {
	// TODO: Consider real validation via reflection for names[-1].
	if len(names) == 0 {
		panic("name / aliases slice must not be empty!")
	}

	var (
		cliFlags    = []cli.Flag{}
		numRequired = 0
	)
	for _, spec := range flagSpecs {
		if len(spec.names) == 0 {
			panic("flag name / aliases slice must not be empty!")
		}
		if spec.required {
			numRequired++
		}
		cliFlags = append(cliFlags, &cli.StringFlag{
			Name:    spec.names[0],
			Aliases: spec.names[1:],
			Usage:   spec.usage,
		})
	}

	return &cli.Command{
		Name:        names[0],
		Aliases:     names[1:],
		Description: description,
		Flags:       cliFlags,
		Action: func(ctx *cli.Context) error {
			var (
				funcArgs     = []interface{}{}
				errs         = []error{}
				argsConsumed = 0
			)
			for _, spec := range flagSpecs {
				val := ctx.String(spec.names[0])
				if spec.required && len(val) == 0 {
					if ctx.Args().Len() > argsConsumed {
						val = ctx.Args().Slice()[argsConsumed]
						argsConsumed++
					}
					if len(val) == 0 {
						errs = append(errs, fmt.Errorf("%v flag is required", spec.names[0]))
					}
				}
				funcArgs = append(funcArgs, val)
			}
			if err := errorlib.Merge(errs); err != nil {
				return err
			}
			return (&core.Client{}).RemoteExec(names[len(names)-1], funcArgs...)
		},
	}
}

var appFlagSpec = flagSpec{
	names:    []string{"app", "a"},
	usage:    "Name of app",
	required: true,
}

// appCommand generates a simple app command.
//
// The names parameter must be non-empty and end with a value which corresponds
// to a valid shipbuilder command function.
func appCommand(names []string, description string, flagSpecs ...flagSpec) *cli.Command {
	return command(names, description, append([]flagSpec{appFlagSpec}, flagSpecs...)...)
}

// argsOrFlagAppCommand generates an app command with a single flag of type
// string slice which can also be passed as unnamed arguments.
//
// The names and flagNames parameters must be non-empty and end with a value
// which corresponds to a valid shipbuilder command function.
func argsOrFlagAppCommand(names []string, description string, flagNames []string, flagUsage string) *cli.Command {
	// TODO: Consider real validation via reflection for names[-1].
	if len(names) == 0 {
		panic("name / aliases slice must not be empty!")
	}
	if len(flagNames) == 0 {
		panic("flag name / aliases slice must not be empty!")
	}
	return &cli.Command{
		Name:        names[0],
		Aliases:     names[1:],
		Description: description,
		Flags: []cli.Flag{
			appFlag,
			&cli.StringSliceFlag{
				Name:    flagNames[0],
				Aliases: flagNames[1:],
				Usage:   flagUsage,
			},
		},
		Action: func(ctx *cli.Context) error {
			var (
				app         = ctx.String("app")
				dynamicFlag = ctx.StringSlice(flagNames[0])
			)
			if len(app) == 0 {
				return errors.New("app flag is required")
			}
			// NB: Notice the precedence here - flag is respected above args.
			if ctx.Args().Present() && len(dynamicFlag) == 0 {
				dynamicFlag = ctx.Args().Slice()
			}
			if len(dynamicFlag) == 0 {
				return fmt.Errorf("%v flag or args values are required", flagNames[0])
			}
			return (&core.Client{}).RemoteExec(names[len(names)-1], app, dynamicFlag)
		},
	}
}

// deferredMappedAppCommand generates a deferred mapped app command.
//
// The names parameter must be non-empty and end with a value which corresponds
// to a valid shipbuilder command function.
func deferredMappedAppCommand(names []string, description string) *cli.Command {
	// TODO: Consider real validation via reflection for names[-1].
	if len(names) == 0 {
		panic("name / aliases slice must not be empty!")
	}
	return &cli.Command{
		Name:        names[0],
		Aliases:     names[1:],
		Description: description,
		Flags: []cli.Flag{
			appFlag,
			&cli.BoolFlag{
				Name:    "deferred",
				Aliases: []string{"defer", "d"},
				Usage:   "Defer app redeployment",
			},
		},
		Action: func(ctx *cli.Context) error {
			var (
				app      = ctx.String("app")
				deferred = ctx.Bool("deferred")
				mapped   = map[string]string{}
				errs     = []error{}
			)
			if len(app) == 0 {
				return errors.New("app flag is required")
			}
			for _, arg := range ctx.Args().Slice() {
				if pieces := strings.SplitN(arg, "=", 2); len(pieces) == 2 {
					mapped[pieces[0]] = pieces[1]
				} else {
					errs = append(errs, fmt.Errorf("malformed arg %q; must be of the form key=value", arg))
				}
			}
			if err := errorlib.Merge(errs); err != nil {
				return err
			}
			if len(mapped) == 0 {
				return errors.New("invalid due to empty map of key/value parameters")
			}
			return (&core.Client{}).RemoteExec(names[len(names)-1], app, deferred, mapped)
		},
	}
}

func buildpackSubcommands() []*cli.Command {
	var (
		cmds     = []*cli.Command{}
		provider = bindata_buildpacks.NewProvider()
	)

	for _, bp := range provider.All() {
		subCmds := []*cli.Command{
			&cli.Command{
				Name:    "container-custom-commands",
				Aliases: []string{"ContainerCustomCommands"},
				Action: func(ctx *cli.Context) error {
					fmt.Fprintf(os.Stdout, "%v\n", bp.ContainerCustomCommands())
					return nil
				},
			},
			&cli.Command{
				Name:    "container-packages",
				Aliases: []string{"ContainerPackages"},
				Action: func(ctx *cli.Context) error {
					fmt.Fprintf(os.Stdout, "%v\n", strings.Join(bp.ContainerPackages(), "\n"))
					return nil
				},
			},
			&cli.Command{
				Name:    "pre-hook",
				Aliases: []string{"PreHook"},
				Action: func(ctx *cli.Context) error {
					fmt.Fprintf(os.Stdout, "%v\n", bp.PreHook())
					return nil
				},
			},
		}
		cmd := &cli.Command{
			Name: bp.Name(),
			Action: func(ctx *cli.Context) error {
				fmt.Fprint(os.Stdout, "container-custom-commands\ncontainer-packages\npre-hook\n")
				return nil
			},
			Subcommands: subCmds,
		}
		cmds = append(cmds, cmd)
	}

	return cmds
}