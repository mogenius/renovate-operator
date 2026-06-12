# Changelog

## [4.11.0](https://github.com/mogenius/renovate-operator/compare/4.10.1...4.11.0) (2026-06-12)


### Features

* **api:** add runtimeClassName to RenovateJobSpec ([8778caa](https://github.com/mogenius/renovate-operator/commit/8778caa979c3bd97b222a54dcb7baded6dc84f41))
* improve label selector on jobs ([6421374](https://github.com/mogenius/renovate-operator/commit/642137449ead20a31815f2fee4e73dd45ab2431a))
* moving discovery jobs to reconciler based processing ([41649a9](https://github.com/mogenius/renovate-operator/commit/41649a9e50ef9d9c24d8f17a47a7b456e3626a74))
* reconcile project jobs via manager instead of loop ([aa118be](https://github.com/mogenius/renovate-operator/commit/aa118be079841e1da22e3c07a80e8d6b55039bb4))
* skip pending-deletion repos during discovery ([a956471](https://github.com/mogenius/renovate-operator/commit/a9564715092825714900e39452705be79c5a18f2))


### Bug Fixes

* add tracing to job reconciler ([514352a](https://github.com/mogenius/renovate-operator/commit/514352ab68892dde6c2b38759f6bcee13bbc6122))
* adding renovatejob reconciler to check for orphaned jobs ([88ec818](https://github.com/mogenius/renovate-operator/commit/88ec818db6b3aa15aa35a1805d3e227a4b925a8f))
* annotate processed jobs to prevent double processing ([a4e10df](https://github.com/mogenius/renovate-operator/commit/a4e10dfea895c539dc57a5bbc63c561781cfd09c))
* clean up mobile view and only display issues or activity if they exist ([930cd42](https://github.com/mogenius/renovate-operator/commit/930cd42f3f677474994bb85404a812300e10817c))
* do not display loading animation on background reload ([47e1fdf](https://github.com/mogenius/renovate-operator/commit/47e1fdff8e4b4ae94b09d9ec9aaca96f47c00c05))

## [4.10.1](https://github.com/mogenius/renovate-operator/compare/4.10.0...4.10.1) (2026-06-03)


### Bug Fixes

* apply fixxes proposed by go fix command ([d6f5e13](https://github.com/mogenius/renovate-operator/commit/d6f5e137bcee8c28c4900a3d22d99673b06f835c))
* delete successful discovery jobs when DELETE_SUCCESSFUL_JOBS=true ([#377](https://github.com/mogenius/renovate-operator/issues/377)) ([1237620](https://github.com/mogenius/renovate-operator/commit/12376203883c24c938b026ee4b45f0d660bb6628))
* **deps:** update golang docker tag to v1.26.4 ([84e7b85](https://github.com/mogenius/renovate-operator/commit/84e7b859024c9bd8721e35ddc8e0677d7f20c447))
* **deps:** update module github.com/golang-jwt/jwt/v5 to v5.3.1 ([9ec93cd](https://github.com/mogenius/renovate-operator/commit/9ec93cdac2c0530afcbedc4bc18d897f0c1fd0dd))
* **deps:** update module github.com/netresearch/go-cron to v0.15.0 ([785db64](https://github.com/mogenius/renovate-operator/commit/785db64658afe91adeeeb630dcad82f81215e0bd))
* replace depracated controller-runtime scheme with apimachinery ([fdef1a3](https://github.com/mogenius/renovate-operator/commit/fdef1a312606663aacc81bba34df9b63ce51f028))

## [4.10.0](https://github.com/mogenius/renovate-operator/compare/4.9.0...4.10.0) (2026-05-29)


### Features

* adding native github app support ([19221c1](https://github.com/mogenius/renovate-operator/commit/19221c1da01f78d4bd44cea17a7b877d41b9a38d))


### Bug Fixes

* **deployment:** valkey: wrong path of usersExistingSecret ([0545fbd](https://github.com/mogenius/renovate-operator/commit/0545fbd4306c8f926ef47cdf72f5c9157584f538))
* honor valkey db if complete valkey url has been set ([332acef](https://github.com/mogenius/renovate-operator/commit/332acef28ca05d99db95137495e074a5ba3c2577)), closes [#364](https://github.com/mogenius/renovate-operator/issues/364)

## [4.9.0](https://github.com/mogenius/renovate-operator/compare/4.8.1...4.9.0) (2026-05-28)


### Features

* **actions:** migrate from semantic release to release-please ([c0ca31a](https://github.com/mogenius/renovate-operator/commit/c0ca31a4a187444facfc2a250d4f49d564f422ac))


### Bug Fixes

* **deployment:** support custom valkey auth secret ([7e983f1](https://github.com/mogenius/renovate-operator/commit/7e983f1247f5476f3825c4d34d6f64b11e7b7551))
* **deps:** update go module directive to v1.26.3 ([ee5accf](https://github.com/mogenius/renovate-operator/commit/ee5accf1cba518cc9a9c9934720711dc807f3db5))
* **deps:** update kubernetes monorepo to v0.36.1 ([245f409](https://github.com/mogenius/renovate-operator/commit/245f4095fbb16223a2480b770174bdae7b0e4276))
* **deps:** update opentelemetry-go monorepo ([9410306](https://github.com/mogenius/renovate-operator/commit/94103066bee9c79e1a3a7dc3c8bcb51672de1619))
* **deps:** update opentelemetry-go-contrib monorepo ([65a9de8](https://github.com/mogenius/renovate-operator/commit/65a9de8400af5da718a09cf438256791c76f609d))
* do not include v in release-please tag ([d1aa683](https://github.com/mogenius/renovate-operator/commit/d1aa683b3750385bf5a90c62788bbdceba50c106))
* **dockerfile:** use all three version parts for the builder image ([fe0920b](https://github.com/mogenius/renovate-operator/commit/fe0920b9a8cfd5638a0a7d6585ce40c3dc9186f3))
