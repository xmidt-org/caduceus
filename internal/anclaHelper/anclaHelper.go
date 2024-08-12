package anclahelper

import (
	"context"
	"fmt"

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/sallust"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type In struct {
	fx.In
	Svc      *ancla.ClientService
	Listener ancla.ListenerConfig
	Sink     sink.Wrapper
	LC       fx.Lifecycle
}

func InitializeAncla(in In) error {

	stopWatches, err := in.Svc.StartListener(in.Listener, setLoggerInContext(), in.Sink)
	if err != nil {
		return fmt.Errorf("webhook service start listener error: %v", err)
	}
	in.LC.Append(fx.StopHook(stopWatches))
	return nil

}

func setLoggerInContext() func(context.Context, *zap.Logger) context.Context {
	return func(parent context.Context, logger *zap.Logger) context.Context {
		return sallust.With(parent, logger)
	}
}
