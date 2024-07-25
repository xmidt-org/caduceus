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

type ServiceListenerIn struct{
	fx.In
	Listener ancla.ListenerConfig
	//This is where we would need to add ancla.service struct
}
func InitializeAncla(lifecycle fx.Lifecycle) fx.Option {
	return fx.Options(

		//svc is the ancla.service struct that is returned when we call ancla.NewService(config, listener)
		//can't have this in ancla as it requires the sink struct (implementing the ancla.Watch interface)
			stopWatches, err := svc.StartListener(in.Listener, setLoggerInContext(), in.Sink)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Webhook service start listener error: %v\n", err)
				return 1
			}
			lifecycle.Append(fx.StopHook(stopWatches))
			return 0
		}
	),
}

func setLoggerInContext() func(context.Context, *zap.Logger) context.Context {
	return func(parent context.Context, logger *zap.Logger) context.Context {
		return sallust.With(parent, logger)
	}
}
