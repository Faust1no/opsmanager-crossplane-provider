package main

import (
	"os"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
	"github.com/crossplane-contrib/provider-opsmanager/internal/controller/backupdaemon"
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
	zl := zap.New(zap.UseDevMode(true), zap.StacktraceLevel(zapcore.PanicLevel))
	ctrl.SetLogger(zl)
	log := logging.NewLogrLogger(zl.WithName("provider-opsmanager"))

	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Debug("Cannot get API server rest config", "error", err)
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  scheme,
		LeaderElection:          true,
		LeaderElectionID:        "crossplane-provider-opsmanager-leader",
		LeaderElectionNamespace: "crossplane-system",
	})
	if err != nil {
		log.Debug("Cannot create manager", "error", err)
		os.Exit(1)
	}

	o := controller.Options{
		Logger:                  log,
		MaxConcurrentReconciles: 1,
		PollInterval:            1 * time.Minute,
		GlobalRateLimiter:       ratelimiter.NewGlobal(10),
		Features:                &feature.Flags{},
	}

	if err := project.Setup(mgr, o); err != nil {
		log.Debug("Cannot setup Project controller", "error", err)
		os.Exit(1)
	}
	if err := s3blockstore.Setup(mgr, o); err != nil {
		log.Debug("Cannot setup S3Blockstore controller", "error", err)
		os.Exit(1)
	}
	if err := s3oplogstore.Setup(mgr, o); err != nil {
		log.Debug("Cannot setup S3OplogStore controller", "error", err)
		os.Exit(1)
	}
	if err := backupdaemon.Setup(mgr, o); err != nil {
		log.Debug("Cannot setup BackupDaemon controller", "error", err)
		os.Exit(1)
	}

	log.Debug("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Debug("Cannot start manager", "error", err)
		os.Exit(1)
	}
}
