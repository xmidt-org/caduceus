# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.7.0]
- Added zap logger and basculehelper package [#403] (https://github.com/xmidt-org/caduceus/pull/403)

## [v0.6.13]
- Updated tracing configuration documentation in caduceus.yaml to reflect changes in Candlelight [#372](https://github.com/xmidt-org/caduceus/pull/372)
- Updated bascule config in docker env [#388] (https://github.com/xmidt-org/caduceus/pull/388/files)

## [v0.6.12]
- Remove several unused build files and update the docker images to work. [#360](https://github.com/xmidt-org/caduceus/pull/360)

## [v0.6.11]
- Fix the docker container so it has configuration in the right place.

## [v0.6.10]
- Dependency updates with no vulnerabilities patched.
- Fix linting related issues.
- Restore building docker images.

## [v0.6.9]
- Added latency metric, which Tracks the time spent waiting on outbound client URLs to respond. [#312](https://github.com/xmidt-org/caduceus/pull/312)
- Dependency update, note vulnerabilities
  - github.com/hashicorp/consul/api v1.13.1 // indirect
    Wasn't able to find much info about this one besides the following dep vulns
    - golang.org/x/net
      - https://nvd.nist.gov/vuln/detail/CVE-2021-33194
      - https://nvd.nist.gov/vuln/detail/CVE-2021-31525
      - https://nvd.nist.gov/vuln/detail/CVE-2021-44716
  - Introduces new vuln https://www.mend.io/vulnerability-database/CVE-2022-29526
  -  guardrails says github.com/gorilla/websocket v1.5.0 has a high vulnerability but no vulnerabilities have been filed
- JWT Migration #331 
  - updated to use clortho `Resolver` & `Refresher`
  - updated to use clortho `metrics` & `logging`
- Update ancla client initialization
- Update Config
  - Use [uber/zap](https://github.com/uber-go/zap) for clortho logging
  - Use [xmidt-org/sallust](https://github.com/xmidt-org/sallust) for the zap config unmarshalling 
  - Update auth config for clortho
  - Update ancla config

## [v0.6.6]
- Fix a missing return after an invalid utf8 string is handled.  [#315](https://github.com/xmidt-org/caduceus/pull/315)

## [v0.6.5]
- Bumped wrp-go to v3.1.3 in order to reject all wrps with non-utf-8 strings as invalid. [#314](https://github.com/xmidt-org/caduceus/pull/314)
- Modified senderWrapper to defer all lock unlocking. [#314](https://github.com/xmidt-org/caduceus/pull/314)
- Added github.com/xmidt-org/httpaux as a dependency
- Added httpaux/recovery middleware to the top-level gorilla/mux Router to prevent panics from shutting down a server

## [v0.6.4]
- Fixed issue of endpoints no longer being found. [#311](https://github.com/xmidt-org/caduceus/pull/311)

## [v0.6.3]
- Added configurable support for v3 notify endpoint. [#309](https://github.com/xmidt-org/caduceus/pull/309)

## [v0.6.2]
- Updated spec file and rpkg version macro to be able to choose when the 'v' is included in the version. [#304](https://github.com/xmidt-org/caduceus/pull/304)
- Added debug logging to outbound sender. [#305](https://github.com/xmidt-org/caduceus/pull/305)

## [v0.6.1]
- Bump wrp-go version. [#301](https://github.com/xmidt-org/caduceus/pull/301)
- Reject all non-SimpleEvents wrp messages.[#300](https://github.com/xmidt-org/caduceus/pull/300)

## [v0.6.0]
- Removed endpoint for registering webhooks. [#299](https://github.com/xmidt-org/caduceus/pull/299)
- Moved auth middleware to use bascule package instead of deprecated webpa-common/secure. [#299](https://github.com/xmidt-org/caduceus/pull/299)
- Updated api version in url to v4 to indicate breaking changes in response codes when an invalid auth is sent. [#299](https://github.com/xmidt-org/caduceus/pull/299)

## [v0.5.2]
- Edited retry functions in outboundSender and bumped webpa-common from v2.0.1 to v2.0.3. [#280](https://github.com/xmidt-org/caduceus/pull/280)
- Bumped ancla to v0.3.5 in order to fix panic in post webhook endpoint. [#298](https://github.com/xmidt-org/caduceus/pull/298)
- Used already existing configuration for whether to check partner IDs in post webhook endpoint. [#298](https://github.com/xmidt-org/caduceus/pull/298)

## [v0.5.1]
- Edited Queue function to fix [duplicate events bug](https://github.com/xmidt-org/caduceus/issues/277). [#278](https://github.com/xmidt-org/caduceus/pull/278)

## [v0.5.0]
- Bumped webpa-common to v2. [#276](https://github.com/xmidt-org/caduceus/pull/276)
- Added configuration for PartnerID check. [#276](https://github.com/xmidt-org/caduceus/pull/276)
- Changed passsing Webhooks to InternalWebhooks to allow for storage of PartnerIDs. [#276](https://github.com/xmidt-org/caduceus/pull/276)
- Added 415 response for Content-Type header issues. [#272](https://github.com/xmidt-org/caduceus/pull/273)
- Fixed log and response for incoming queue full 503. [#275](https://github.com/xmidt-org/caduceus/pull/275)

## [v0.4.6]
- Prevent Authorization header from getting logged. [#270](https://github.com/xmidt-org/caduceus/pull/270)
- Bumped ancla, webpa-common versions. [#271](https://github.com/xmidt-org/caduceus/pull/271)

## [v0.4.5]
- Add support for acquiring Themis tokens through Ancla. [#267](https://github.com/xmidt-org/caduceus/pull/267)
- Update candlelight version. [#267](https://github.com/xmidt-org/caduceus/pull/267)

## [v0.4.4]
- Add OpenTelemetry tracing feature. [#263](https://github.com/xmidt-org/caduceus/pull/263) thanks to @utsavbatra5

## [v0.4.3]
### Changed
- Fix a bug where the response bodies were not cleaned up when informing a client of a failure. [#250](https://github.com/xmidt-org/caduceus/pull/250)
- Migrate to github actions, normalize analysis tools, Dockerfiles and Makefiles. [#246](https://github.com/xmidt-org/caduceus/pull/246)
- Update buildtime format in Makefile to match RPM spec file. [#245](https://github.com/xmidt-org/caduceus/pull/245)
- Update to use wrp-go v3 to be able to use the MIME constants. [#252](https://github.com/xmidt-org/caduceus/pull/252)
- Remove some unused code. [#254](https://github.com/xmidt-org/caduceus/pull/254)
- Bump webpa-common version with xwebhook item ID format update. [#257](https://github.com/xmidt-org/caduceus/pull/257)
- Update webhook integration to use xmidt-org/ancla. [#259](https://github.com/xmidt-org/caduceus/pull/259)

## [v0.4.2]
### Fixed
- Bug in which only mTLS was allowed as valid config for a webpa server. [#242](https://github.com/xmidt-org/caduceus/pull/242) 

## [v0.4.1]
- update argus integration [#239](https://github.com/xmidt-org/caduceus/pull/239)
- switch webhook configuration from sns to argus [#202](https://github.com/xmidt-org/caduceus/pull/202)
- removed `/hooks` endpoint [#202](https://github.com/xmidt-org/caduceus/pull/202)
- Updated references to the main branch [#227](https://github.com/xmidt-org/caduceus/pull/227)
- Fixed bug of chrysom client not starting [#235](https://github.com/xmidt-org/caduceus/pull/235)
- Empty queue when webhook expires [#237](https://github.com/xmidt-org/caduceus/pull/237)

## [v0.4.0]
- Moved and renamed configuration variable for outgoing hostname validation [#223](https://github.com/xmidt-org/caduceus/pull/223)

## [v0.3.0]
- added metric for counting when caduceus re-encodes the wrp [#216](https://github.com/xmidt-org/caduceus/pull/216)
- Made outgoing hostname validation configurable [#217](https://github.com/xmidt-org/caduceus/pull/217)
  - **Note:** To be backwards compatable, the configuration value of `allowInsecureTLS: true` will need to be defined, otherwise hostname validation is enabled by default.
- removed contentTypeCounter [#218](https://github.com/xmidt-org/caduceus/pull/218)
- added configuration for which http codes Caduceus should retry on [#219](https://github.com/xmidt-org/caduceus/pull/219)
  - **Note:** This configuration change causes the existing retry logic to change.

## [v0.2.8]
### Changed
- cleaned up shutdown logic for outbound sender [#205](https://github.com/xmidt-org/caduceus/pull/205)
- added resetting queue depth and current workers gauges to outbound sender [#205](https://github.com/xmidt-org/caduceus/pull/205)
- removed queueEmpty variable from outbound sender [#209](https://github.com/xmidt-org/caduceus/pull/209)
- fixed outbound sender's long running dispatcher() goroutine to not exit when a cutoff occurs [#210](https://github.com/xmidt-org/caduceus/pull/210)
- register for specific OS signals [#211](https://github.com/xmidt-org/caduceus/pull/211)

## [v0.2.7]
- pared down logging, especially debugging logs [#196](https://github.com/xmidt-org/caduceus/pull/196)
- added dropped events to metric [#195](https://github.com/xmidt-org/caduceus/issues/195)
- removed all calls to logging.Debug(), logging.Info(), and logging.Error() [#199](https://github.com/xmidt-org/caduceus/pull/199)
- bumped webpa-common version to use a webhooks page without those logging calls [#199](https://github.com/xmidt-org/caduceus/pull/199)
- bumped webpa-common version includes a fix to authorization logging issue [#192](https://github.com/xmidt-org/caduceus/issues/192)

## [v0.2.6]
- reduced time from when cutoff is sent to when queue is emptied

## [v0.2.5]
- fix emptying queue when received cutoff [#188](https://github.com/xmidt-org/caduceus/issues/188)
- add queue full check to prevent push event into queue if already full [#189](https://github.com/xmidt-org/caduceus/issues/189)

## [v0.2.4]
- added docker automation [#184](https://github.com/xmidt-org/caduceus/pull/184)
- fixed caduceus queue cutoffs [#185](https://github.com/xmidt-org/caduceus/issues/185)

## [v0.2.3]
- updated release pipeline to use travis [#174](https://github.com/xmidt-org/caduceus/pull/174)
- bumped wrp-go to v2 [#182](https://github.com/xmidt-org/caduceus/pull/182)

## [v0.2.2]
- bump webpa-common to v1.5.0

## [v0.2.1]
### Changed
- Fixed a missing cardinality dimension in a metric that caused a panic.

## [v0.2.0]
### Added
- Metrics to support debugging the problem found by GH Issue [issue 145](https://github.com/xmidt-org/caduceus/issues/145)
- Add WRP Header support for Partner-Ids and Metadata

### Changed
- converting glide to go mod

## [v0.1.5]
fixed build upload

## [v0.1.4]
### Added
- Add alternative urls and consumer max retry logic for webhooks

### Changed
- Retry on non 2xx status codes
- Fix for no retries being attempted
- Add metric for incoming content type

## [v0.1.3] - 2019-02-27
### Changed
- Fix for [issue 126](https://github.com/xmidt-org/caduceus/issues/126)

## [v0.1.2] - 2019-02-21
### Added
- Fix for delivering events as json or msgpack based events [issue 113](https://github.com/xmidt-org/caduceus/issues/113)

### Changed
- Updated to new version of webpa-common library
- Remove the worker pool as a fixed number of workers per endpoint and simply cap
  the maximum number.  Partial fix for [issue 115](https://github.com/xmidt-org/caduceus/issues/115), [issue 103](https://github.com/xmidt-org/caduceus/issues/103)
- Fix for webhook shallow copy bug.  Partial fix for [issue 115](https://github.com/xmidt-org/caduceus/issues/115)
- Fix for webhook update for all fields (updated webpa-common code to bring in fix)
- Fix for retry logic so all failures are retried the specified number of times - [issue 91](https://github.com/xmidt-org/caduceus/issues/91)
- Fix for waiting for DNS to resolve prior to listening for webhook updates - [issue 111](https://github.com/xmidt-org/caduceus/issues/111)
- Fix for cpu spike after about 10 mintues due to worker go routines not finishing.
- Fix logic for updating webhooks
- Fix for sending the same event multiple times to the same webhook.
- Fix for [issue 99](https://github.com/xmidt-org/caduceus/issues/99)

## [0.1.1] - 2018-04-06
### Added
- Fix for X-Webpa-Event header
- Use all cores-1 for IO control by default
- Fix a bug where the regex matching was too greedy
- Add retries on errors deemed "retryable"
- Add all X-Midt headers
- Add metrics data

## [0.0.1] - 2017-03-28
### Added
- Initial creation

[Unreleased]: https://github.com/xmidt-org/caduceus/compare/v0.7.0...HEAD
[v0.7.0]: https://github.com/xmidt-org/caduceus/compare/v0.6.13...v0.7.0
[v0.6.13]: https://github.com/xmidt-org/caduceus/compare/v0.6.12...v0.6.13
[v0.6.12]: https://github.com/xmidt-org/caduceus/compare/v0.6.11...v0.6.12
[v0.6.11]: https://github.com/xmidt-org/caduceus/compare/v0.6.10...v0.6.11
[v0.6.10]: https://github.com/xmidt-org/caduceus/compare/v0.6.9...v0.6.10
[v0.6.9]: https://github.com/xmidt-org/caduceus/compare/v0.6.6...v0.6.9
[v0.6.6]: https://github.com/xmidt-org/caduceus/compare/v0.6.5...v0.6.6
[v0.6.5]: https://github.com/xmidt-org/caduceus/compare/v0.6.4...v0.6.5
[v0.6.4]: https://github.com/xmidt-org/caduceus/compare/v0.6.3...v0.6.4
[v0.6.3]: https://github.com/xmidt-org/caduceus/compare/v0.6.2...v0.6.3
[v0.6.2]: https://github.com/xmidt-org/caduceus/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/xmidt-org/caduceus/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/xmidt-org/caduceus/compare/v0.5.2...v0.6.0
[v0.5.2]: https://github.com/xmidt-org/caduceus/compare/v0.5.1...v0.5.2
[v0.5.1]: https://github.com/xmidt-org/caduceus/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/xmidt-org/caduceus/compare/v0.4.6...v0.5.0
[v0.4.6]: https://github.com/xmidt-org/caduceus/compare/v0.4.5...v0.4.6
[v0.4.5]: https://github.com/xmidt-org/caduceus/compare/v0.4.4...v0.4.5
[v0.4.4]: https://github.com/xmidt-org/caduceus/compare/v0.4.3...v0.4.4
[v0.4.3]: https://github.com/xmidt-org/caduceus/compare/v0.4.2...v0.4.3
[v0.4.2]: https://github.com/xmidt-org/caduceus/compare/v0.4.1...v0.4.2
[v0.4.1]: https://github.com/xmidt-org/caduceus/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/xmidt-org/caduceus/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/xmidt-org/caduceus/compare/v0.2.8...v0.3.0
[v0.2.8]: https://github.com/xmidt-org/caduceus/compare/v0.2.7...v0.2.8
[v0.2.7]: https://github.com/xmidt-org/caduceus/compare/v0.2.6...v0.2.7
[v0.2.6]: https://github.com/xmidt-org/caduceus/compare/v0.2.5...v0.2.6
[v0.2.5]: https://github.com/xmidt-org/caduceus/compare/v0.2.4...v0.2.5
[v0.2.4]: https://github.com/xmidt-org/caduceus/compare/v0.2.3...v0.2.4
[v0.2.3]: https://github.com/xmidt-org/caduceus/compare/v0.2.2...v0.2.3
[v0.2.2]: https://github.com/xmidt-org/caduceus/compare/v0.2.2-rc.1...v0.2.2
[v0.2.1]: https://github.com/xmidt-org/caduceus/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/xmidt-org/caduceus/compare/v0.1.5...v0.2.0
[v0.1.5]: https://github.com/xmidt-org/caduceus/compare/v0.1.4...v0.1.5
[v0.1.4]: https://github.com/xmidt-org/caduceus/compare/v0.1.3...v0.1.4
[v0.1.3]: https://github.com/xmidt-org/caduceus/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/xmidt-org/caduceus/compare/0.1.1...v0.1.2
[0.1.1]: https://github.com/xmidt-org/caduceus/compare/0.0.1...0.1.1
[0.0.1]: https://github.com/xmidt-org/caduceus/compare/v0.0.0...0.0.1
