// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package handler

const (
	apiVersion         = "v4"
	prevAPIVersion     = "v3"
	apiBase            = "api/" + apiVersion
	apiBaseDualVersion = "api/{version:" + apiVersion + "|" + prevAPIVersion + "}"
)

type CapabilityConfig struct {
	Type            string
	Prefix          string
	AcceptAllMethod string
	EndpointBuckets []string
}

// func NewPrimaryHandler(l *zap.Logger, v *viper.Viper, sw *ServerHandler, router *mux.Router, prevVersionSupport bool) (*mux.Router, error) {
// 	auth, err := authenticationMiddleware(v, l)
// 	if err != nil {
// 		// nolint:errorlint
// 		return nil, fmt.Errorf("unable to build authentication middleware: %v", err)
// 	}

// 	// if we want to support the previous API version, then include it in the
// 	// api base.
// 	urlPrefix := fmt.Sprintf("/%s", apiBase)
// 	if prevVersionSupport {
// 		urlPrefix = fmt.Sprintf("/%s", apiBaseDualVersion)
// 	}

// 	router.Handle(urlPrefix+"/notify", auth.Then(sw)).Methods("POST")

// 	return router, nil
// }

// // authenticationMiddleware configures the authorization requirements for requests to reach the main handler
// func authenticationMiddleware(v *viper.Viper, logger *zap.Logger) (*alice.Chain, error) {

// 	// basculeMeasures := basculehelper.NewAuthValidationMeasures(registry)
// 	// capabilityCheckMeasures := basculehelper.NewAuthCapabilityCheckMeasures(registry)
// 	// listener := basculehelper.NewMetricListener(basculeMeasures)

// 	basicAllowed := make(map[string]string)
// 	basicAuth := v.GetStringSlice("authHeader")
// 	for _, a := range basicAuth {
// 		decoded, err := base64.StdEncoding.DecodeString(a)
// 		if err != nil {
// 			logger.Info("failed to decode auth header", zap.String("authHeader", a), zap.Error(err))
// 			continue
// 		}

// 		i := bytes.IndexByte(decoded, ':')
// 		logger.Debug("decoded string", zap.ByteString("string", decoded), zap.Int("i", i))
// 		if i > 0 {
// 			basicAllowed[string(decoded[:i])] = string(decoded[i+1:])
// 		}
// 	}
// 	logger.Debug("Created list of allowed basic auths", zap.Any("allowed", basicAllowed), zap.Any("config", basicAuth))

// 	options := []basculehttp.COption{
// 		basculehttp.WithCLogger(logging.GetLogger),
// 		// basculehttp.WithCErrorResponseFunc(listener.OnErrorResponse),
// 	}
// 	if len(basicAllowed) > 0 {
// 		options = append(options, basculehttp.WithTokenFactory("Basic", basculehttp.BasicTokenFactory(basicAllowed)))
// 	}

// 	var jwtVal JWTValidator
// 	// Get jwt configuration, including clortho's configuration
// 	v.UnmarshalKey("jwtValidator", &jwtVal)
// 	kr := clortho.NewKeyRing()

// 	// Instantiate a fetcher for refresher and resolver to share
// 	f, err := clortho.NewFetcher()
// 	if err != nil {
// 		return &alice.Chain{}, emperror.With(err, "failed to create clortho fetcher")
// 	}

// 	ref, err := clortho.NewRefresher(
// 		clortho.WithConfig(jwtVal.Config),
// 		clortho.WithFetcher(f),
// 	)
// 	if err != nil {
// 		return &alice.Chain{}, emperror.With(err, "failed to create clortho refresher")
// 	}

// 	resolver, err := clortho.NewResolver(
// 		clortho.WithConfig(jwtVal.Config),
// 		clortho.WithKeyRing(kr),
// 		clortho.WithFetcher(f),
// 	)
// 	if err != nil {
// 		return &alice.Chain{}, emperror.With(err, "failed to create clortho resolver")
// 	}

// 	var (
// 		tsConfig touchstone.Config
// 		zConfig  sallust.Config
// 	)
// 	// Get touchstone & zap configurations
// 	v.UnmarshalKey("touchstone", &tsConfig)
// 	v.UnmarshalKey("zap", &zConfig)
// 	zlogger := zap.Must(zConfig.Build())
// 	// tf := touchstone.NewFactory(tsConfig, zlogger, promReg)
// 	// Instantiate a metric listener for refresher and resolver to share
// 	// cml, err := clorthometrics.NewListener(clorthometrics.WithFactory(tf))
// 	// if err != nil {
// 	// 	return &alice.Chain{}, emperror.With(err, "failed to create clortho metrics listener")
// 	// }

// 	// Instantiate a logging listener for refresher and resolver to share
// 	czl, err := clorthozap.NewListener(
// 		clorthozap.WithLogger(zlogger),
// 	)
// 	if err != nil {
// 		return &alice.Chain{}, emperror.With(err, "failed to create clortho zap logger listener")
// 	}

// 	// resolver.AddListener(cml)
// 	resolver.AddListener(czl)
// 	// ref.AddListener(cml)
// 	ref.AddListener(czl)
// 	ref.AddListener(kr)
// 	// context.Background() is for the unused `context.Context` argument in refresher.Start
// 	ref.Start(context.Background())
// 	// Shutdown refresher's goroutines when SIGTERM
// 	sigs := make(chan os.Signal, 1)
// 	signal.Notify(sigs, syscall.SIGTERM)
// 	go func() {
// 		<-sigs
// 		// context.Background() is for the unused `context.Context` argument in refresher.Stop
// 		ref.Stop(context.Background())
// 	}()

// 	options = append(options, basculehttp.WithTokenFactory("Bearer", basculehttp.BearerTokenFactory{
// 		DefaultKeyID: "current",
// 		Resolver:     resolver,
// 		Parser:       bascule.DefaultJWTParser,
// 		Leeway:       jwtVal.Leeway,
// 	}))
// 	authConstructor := basculehttp.NewConstructor(append([]basculehttp.COption{
// 		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc("/"+apiBase+"/", basculehttp.DefaultParseURLFunc)),
// 	}, options...)...)
// 	authConstructorLegacy := basculehttp.NewConstructor(append([]basculehttp.COption{
// 		basculehttp.WithParseURLFunc(basculehttp.CreateRemovePrefixURLFunc("/api/"+prevAPIVersion+"/", basculehttp.DefaultParseURLFunc)),
// 		basculehttp.WithCErrorHTTPResponseFunc(basculehttp.LegacyOnErrorHTTPResponse),
// 	}, options...)...)
// 	bearerRules := bascule.Validators{
// 		basculechecks.NonEmptyPrincipal(),
// 		basculechecks.NonEmptyType(),
// 		basculechecks.ValidType([]string{"jwt"}),
// 	}

// 	// only add capability check if the configuration is set
// 	var capabilityCheck CapabilityConfig
// 	v.UnmarshalKey("capabilityCheck", &capabilityCheck)
// 	if capabilityCheck.Type == "enforce" || capabilityCheck.Type == "monitor" {
// 		var endpoints []*regexp.Regexp
// 		c, err := basculehelper.NewEndpointRegexCheck(capabilityCheck.Prefix, capabilityCheck.AcceptAllMethod)
// 		if err != nil {

// 			// nolint:errorlint
// 			return nil, fmt.Errorf("failed to create capability check: %v", err)
// 		}
// 		for _, e := range capabilityCheck.EndpointBuckets {
// 			r, err := regexp.Compile(e)
// 			if err != nil {
// 				logger.Error("failed to compile regular expression", zap.Any("regex", e), zap.Error(err))
// 				continue
// 			}
// 			endpoints = append(endpoints, r)
// 		}
// 		m := basculehelper.MetricValidator{
// 			C:         basculehelper.CapabilitiesValidator{Checker: c},
// 			Endpoints: endpoints,
// 			// Measures:  capabilityCheckMeasures,
// 		}
// 		bearerRules = append(bearerRules, m.CreateValidator(capabilityCheck.Type == "enforce"))
// 	}

// 	authEnforcer := basculehttp.NewEnforcer(
// 		basculehttp.WithELogger(logging.GetLogger),
// 		basculehttp.WithRules("Basic", bascule.Validators{
// 			basculechecks.AllowAll(),
// 		}),
// 		basculehttp.WithRules("Bearer", bearerRules),
// 		// basculehttp.WithEErrorResponseFunc(listener.OnErrorResponse),
// 	)

// 	authChain := alice.New(logging.SetLogger(logger), authConstructor, authEnforcer)             //removing: basculehttp.NewListenerDecorator(listener). commenting for now in case needed later
// 	authChainLegacy := alice.New(logging.SetLogger(logger), authConstructorLegacy, authEnforcer) //removing: basculehttp.NewListenerDecorator(listener) commenting for now in case needed later

// 	versionCompatibleAuth := alice.New(func(next http.Handler) http.Handler {
// 		return http.HandlerFunc(func(r http.ResponseWriter, req *http.Request) {
// 			vars := mux.Vars(req)
// 			if vars != nil {
// 				if vars["version"] == prevAPIVersion {
// 					authChainLegacy.Then(next).ServeHTTP(r, req)
// 					return
// 				}
// 			}
// 			authChain.Then(next).ServeHTTP(r, req)
// 		})
// 	})
// 	return &versionCompatibleAuth, nil
// }
