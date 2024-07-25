package anclahelper

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/sallust"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type AnclaListenerIn struct {
	fx.In
	Measures ancla.Measures
	Logger   *zap.Logger
}
type AnclaServiceIn struct {
	fx.In
	Config   ancla.Config
	Listener ancla.ListenerConfig
	Sink     sink.Wrapper
}

func InitializeAncla(lifecycle fx.Lifecycle) fx.Option {
	return fx.Provide(
		func(in AnclaListenerIn) ancla.ListenerConfig {
			listener := ancla.ListenerConfig{
				Measures: in.Measures,
				Logger:   in.Logger,
			}
			return listener
		},
		func(in AnclaServiceIn) int {
			svc, err := ancla.NewService(in.Config, getLogger)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Webhook service initialization error: %v\n", err)
				return 1
			}

			stopWatches, err := svc.StartListener(in.Listener, setLoggerInContext(), in.Sink)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Webhook service start listener error: %v\n", err)
				return 1
			}
			lifecycle.Append(fx.StopHook(stopWatches))
			return 0
		},
	)
}

func getLogger(ctx context.Context) *zap.Logger {
	logger := sallust.Get(ctx).With(zap.Time("ts", time.Now().UTC()), zap.Any("caller", zap.WithCaller(true)))
	return logger
}

func setLoggerInContext() func(context.Context, *zap.Logger) context.Context {
	return func(parent context.Context, logger *zap.Logger) context.Context {
		return sallust.With(parent, logger)
	}
}
