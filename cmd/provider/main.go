// Package main is the entrypoint for the Ops Manager Crossplane provider.
//
// CLI flags:
//
//	--debug                Enable debug-level logs.
//	--sync-interval        How often to re-reconcile each managed resource.
//	--poll-interval        How often to poll Ops Manager when a resource is up-to-date.
//	--leader-election      Enable leader election (default true).
//	--max-reconcile-rate   Max concurrent reconciles per controller.
//	--log-file             Optional path to also write logs to. Rotation is enabled.
//	--log-file-max-size    Max size in MB of one log file before rotation.
//	--log-file-max-backups Max number of rotated log files to retain.
//	--log-file-max-age     Max age in days of rotated log files.
package main

import (
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
	"github.com/crossplane-contrib/provider-opsmanager/internal/controller/backupdaemon"
	"github.com/crossplane-contrib/provider-opsmanager/internal/controller/config"
	"github.com/crossplane-contrib/provider-opsmanager/internal/controller/project"
	"github.com/crossplane-contrib/provider-opsmanager/internal/controller/s3blockstore"
	"github.com/crossplane-contrib/provider-opsmanager/internal/controller/s3oplogstore"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(v1beta1.SchemeBuilder.AddToScheme(scheme))
}

func main() {
	app := kingpin.New("provider-opsmanager", "Crossplane provider for MongoDB Ops Manager.").
		DefaultEnvars()

	debug := app.Flag("debug", "Run with debug logging.").Short('d').Bool()
	syncInterval := app.Flag("sync-interval", "How often all resources will be double-checked for drift from the desired state.").
		Default("1h").Duration()
	pollInterval := app.Flag("poll-interval", "How often individual resources will be checked for drift from the desired state.").
		Default("1m").Duration()
	leaderElection := app.Flag("leader-election", "Use leader election for the controller manager.").
		Short('l').Default("true").Envar("LEADER_ELECTION").Bool()
	maxReconcileRate := app.Flag("max-reconcile-rate", "The global maximum rate per second at which resources may be reconciled.").
		Default("10").Int()

	logFile := app.Flag("log-file", "Optional file path to mirror logs to (in addition to stderr). Rotated by size/age.").
		Default("").Envar("LOG_FILE").String()
	logFileMaxSize := app.Flag("log-file-max-size", "Maximum size in MB of a log file before it is rotated.").
		Default("100").Int()
	logFileMaxBackups := app.Flag("log-file-max-backups", "Maximum number of rotated log files to retain.").
		Default("5").Int()
	logFileMaxAge := app.Flag("log-file-max-age", "Maximum age in days to retain rotated log files.").
		Default("30").Int()

	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Build the zap logger. By default it writes to stderr at info level.
	// --debug bumps it to debug. If --log-file is set we mirror to a rotating file.
	zopts := []zap.Opts{
		zap.UseDevMode(true),
		zap.StacktraceLevel(zapcore.PanicLevel),
	}
	if *debug {
		zopts = append(zopts, zap.Level(zapcore.DebugLevel))
	} else {
		zopts = append(zopts, zap.Level(zapcore.InfoLevel))
	}
	if *logFile != "" {
		w := &lumberjack.Logger{
			Filename:   *logFile,
			MaxSize:    *logFileMaxSize,
			MaxBackups: *logFileMaxBackups,
			MaxAge:     *logFileMaxAge,
			Compress:   true,
		}
		zopts = append(zopts, zap.WriteTo(w))
	}
	zl := zap.New(zopts...)
	ctrl.SetLogger(zl)
	log := logging.NewLogrLogger(zl.WithName("provider-opsmanager"))

	log.Info("Starting provider",
		"sync-interval", syncInterval.String(),
		"poll-interval", pollInterval.String(),
		"max-reconcile-rate", *maxReconcileRate,
		"leader-election", *leaderElection,
		"log-file", *logFile)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Info("Cannot get API server rest config", "error", err.Error())
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  scheme,
		LeaderElection:          *leaderElection,
		LeaderElectionID:        "crossplane-provider-opsmanager-leader",
		LeaderElectionNamespace: "crossplane-system",
		Cache: cache.Options{
			SyncPeriod: syncInterval,
		},
	})
	if err != nil {
		log.Info("Cannot create controller manager", "error", err.Error())
		os.Exit(1)
	}

	o := controller.Options{
		Logger:                  log,
		MaxConcurrentReconciles: *maxReconcileRate,
		PollInterval:            *pollInterval,
		GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
		Features:                &feature.Flags{},
	}

	setups := []struct {
		name string
		fn   func(ctrl.Manager, controller.Options) error
	}{
		{"ClusterProviderConfig/ProviderConfig", config.Setup},
		{"OpsManagerProject", project.Setup},
		{"S3Blockstore", s3blockstore.Setup},
		{"S3OplogStore", s3oplogstore.Setup},
		{"BackupDaemon", backupdaemon.Setup},
	}
	for _, s := range setups {
		if err := s.fn(mgr, o); err != nil {
			log.Info("Cannot setup controller", "controller", s.name, "error", err.Error())
			os.Exit(1)
		}
		log.Info("Registered controller", "controller", s.name)
	}

	log.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Info("Manager exited with error", "error", err.Error())
		os.Exit(1)
	}
}
