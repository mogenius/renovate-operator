# Changelog

## [6.1.0](https://github.com/mogenius/renovate-operator/compare/6.0.1...6.1.0) (2026-09-01)


### Features

* add annotation based trigger to the renovateproject ([482c6c7](https://github.com/mogenius/renovate-operator/commit/482c6c7a67d59b35f222797284e82fac646ce4d2))
* extract project status into a separate crd per project ([fb2bccf](https://github.com/mogenius/renovate-operator/commit/fb2bccf1f3aa8256bab680a9f19a6e05b9ae1740)), closes [#616](https://github.com/mogenius/renovate-operator/issues/616)


### Bug Fixes

* **chart:** adding permissions for renovateprojects ([bc08342](https://github.com/mogenius/renovate-operator/commit/bc08342b184b88801814dfecf365930aaf0f0152))
* **chart:** install every CRD from the hook, not just renovatejobs ([eebd949](https://github.com/mogenius/renovate-operator/commit/eebd949ab491ffcde4016c44139ae2fbc6b5c289))
* **deps:** update aws-sdk-go-v2 monorepo ([040d7ce](https://github.com/mogenius/renovate-operator/commit/040d7ce78f8df50fabcabb1828a0e354dc33d1d2))
* **deps:** update go to 1.27.0 ([8aeb903](https://github.com/mogenius/renovate-operator/commit/8aeb903a72749ff2683462c54252494eebbdfa82))
* **deps:** update kubernetes monorepo to v0.36.4 ([7763702](https://github.com/mogenius/renovate-operator/commit/7763702bc5f788cf0d031c2ee82ad972772840b8))
* **deps:** update kubernetes monorepo to v0.37.0 ([d1993b2](https://github.com/mogenius/renovate-operator/commit/d1993b2c0252977a169241aaccd7a370c445f0e9))
* **deps:** update module github.com/aws/aws-sdk-go-v2/config to v1.33.2 ([e990156](https://github.com/mogenius/renovate-operator/commit/e990156e900f5cb783d19a83d65f355d30379ef3))
* **deps:** update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.110.0 ([105ee2b](https://github.com/mogenius/renovate-operator/commit/105ee2b9663def2e4cabe069f8d7a06a98f0323d))
* **deps:** update module github.com/netresearch/go-cron to v0.16.0 ([ef0d2e1](https://github.com/mogenius/renovate-operator/commit/ef0d2e135446b1605fa008b7fd01dbd03e5f772d))
* **deps:** update node.js to v24.20.0 ([b4703c5](https://github.com/mogenius/renovate-operator/commit/b4703c56af9968cdded947346992e45352dbca81))
* **deps:** update opentelemetry ([eb1d0f5](https://github.com/mogenius/renovate-operator/commit/eb1d0f5a2ca59634d4a12dd540a92ca9a8fdce44))
* **deps:** update registry.k8s.io/kubectl docker tag to v1.37.0 ([7cda93b](https://github.com/mogenius/renovate-operator/commit/7cda93b60e88b1133bc82aec274de502d1d7ea07))
* do not log partial pem data if pem is not valid ([ccf8985](https://github.com/mogenius/renovate-operator/commit/ccf89850615e0a39f97190f091955dc9db9b5f26))
* forbid usage of github apps if provider.name != github ([7105f19](https://github.com/mogenius/renovate-operator/commit/7105f19469436d3147bc16032f16991601627731))
* golang 1.27 improvements ([019749b](https://github.com/mogenius/renovate-operator/commit/019749bbd587dd0caf0fea4a03f18cd8b4d8897e))
* keep PR activity metrics when a run aborts with repository-changed ([a174da2](https://github.com/mogenius/renovate-operator/commit/a174da244dfc2ba320c9eb90c714f5d5f75ebaec))
* make crd project field immutable ([0b98c6a](https://github.com/mogenius/renovate-operator/commit/0b98c6a6da80b9098b21448ced8947671b8e1aed))
* **otel:** minor tracing improvements ([2536349](https://github.com/mogenius/renovate-operator/commit/25363493a20d35297ac1e1060734a5ae5eedb92e))
* possible sync errors in discovery agent ([53baee4](https://github.com/mogenius/renovate-operator/commit/53baee49ea15841a3818a3969349a41e0fed9d9c))
* return StatusUnauthorized regardless of a matching job ([0ddb1b9](https://github.com/mogenius/renovate-operator/commit/0ddb1b95ca130724771ec5db01d817b4db512a95))
* **ui:** errors if the ui reloads while the tooltip is visible ([dd6a318](https://github.com/mogenius/renovate-operator/commit/dd6a31850a511f0bb35538da44e9f0c7e0a6b41b))
* **ui:** stop the project table overflowing in Chrome ([9b93963](https://github.com/mogenius/renovate-operator/commit/9b9396375d0e754ee08a3a589c29d7322210ab01))

## [6.0.1](https://github.com/mogenius/renovate-operator/compare/6.0.0...6.0.1) (2026-08-20)


### Bug Fixes

* adding list and watch for configmaps ([acca1b9](https://github.com/mogenius/renovate-operator/commit/acca1b9874457965d4f2ace065c8df040a5a960a))
* badges clipped at top on hover ([d1a7365](https://github.com/mogenius/renovate-operator/commit/d1a73657fac527303d3bcf218592d34648484c11)), closes [#606](https://github.com/mogenius/renovate-operator/issues/606)
* **deps:** update aws-sdk-go-v2 monorepo ([35b2e4f](https://github.com/mogenius/renovate-operator/commit/35b2e4f9ad89ba60141fb02ca1ac678c7171a20f))
* **deps:** update golang docker tag to v1.27.0 ([5d33f66](https://github.com/mogenius/renovate-operator/commit/5d33f668f070ce381c989e85081d4cf2c7859547))
* **deps:** update module github.com/valkey-io/valkey-go to v1.0.77 ([b547b87](https://github.com/mogenius/renovate-operator/commit/b547b8701d4c97967f8d7b2fdd0db37d523c810c))
* **deps:** update registry.k8s.io/kubectl docker tag to v1.36.4 ([3fb072f](https://github.com/mogenius/renovate-operator/commit/3fb072ffe56cbf6a8f7edd4b8dd1c60edcb8fd0b))
* **otel:** semconv sdk mismatch ([a002ecf](https://github.com/mogenius/renovate-operator/commit/a002ecf8a7c2d7486fa98a4049796be517e5fa72))
* reduce amount of unnecessary reconciler loops ([5df6b37](https://github.com/mogenius/renovate-operator/commit/5df6b3735361a99717cb02fe432f03b41e0ec59d))
* save the hash of the renovate inline config to reduce api calls ([2f3d7db](https://github.com/mogenius/renovate-operator/commit/2f3d7db972041323975852d5ec1d6e7dc7e23f3e))

## [6.0.0](https://github.com/mogenius/renovate-operator/compare/5.6.0...6.0.0) (2026-08-14)


### ⚠ BREAKING CHANGES

* **auth:** authorization is on by default when authentication is activated. If you're running the operator in an environment where authorization isn't needed but you still want to use authentication, e. G.: in your homelab, you can disable it by setting authorization.enabled = false (or AUTHORIZATION_ENABLED='false' env var) to rely soley on authentication
* **ui:** A RenovateJob with no access configuration is now hidden from everyone once UI authentication is enabled, where before it was visible to every authenticated user. Installs without an auth provider are unaffected. GitHub OAuth installs using groups must set auth.github.orgGroups=true. See the migration guide at docs/migration/migration-v5-to-v6.md for more info.
* The deprecated job identifiers are now removed. The renovate-operator actively migrated them since quite a few versions so you shouldn't be affected but we still marked it as breaking change.
* **security:** RenovateJobs may no longer declare hostPath volumes, privileged containers or allowPrivilegeEscalation. The API server rejects them, and existing objects using them become unapplyable. The rest of the added policy engine is purely optional and disabled by default. Look at the migration guide docs/migration/migration-v5-to-v6.md for further explanations.

### Features

* **auth:** add authorization master switch ([35e7bdd](https://github.com/mogenius/renovate-operator/commit/35e7bdd189cdb00cfae81f8f39cd6c0a07fa133f))
* **github-app:** promote native github integration from beta to stable ([be77ce5](https://github.com/mogenius/renovate-operator/commit/be77ce5e977785f6292ecc4e0e52d0b4796dbac5))
* **operator:** support inline and configmap-based renovate config ([2d3df52](https://github.com/mogenius/renovate-operator/commit/2d3df526437fc1e548d4b787d0f2f964d5bfe4c4))
* replace persistent debug with single push debug ([225d516](https://github.com/mogenius/renovate-operator/commit/225d5161418d80b119fbfc058ef15a86ed3ae5cc)), closes [#561](https://github.com/mogenius/renovate-operator/issues/561)
* **security:** add optional policy engine to harden renovate runs ([1f0caf2](https://github.com/mogenius/renovate-operator/commit/1f0caf2d34a46deb8dfc1faa17cec4d9a25c9421))
* **ui:** add access model for reader, admins, and anonymous access ([09b9e6e](https://github.com/mogenius/renovate-operator/commit/09b9e6eca04e9aba06c529f18cf8359cd23f9fd6)), closes [#380](https://github.com/mogenius/renovate-operator/issues/380)
* **ui:** pin toolbar and search to the top ([#572](https://github.com/mogenius/renovate-operator/issues/572)) ([9e894f1](https://github.com/mogenius/renovate-operator/commit/9e894f15b821b5f511f00525e9701c83463575c0))


### Bug Fixes

* **auth:** match the OIDC group filter case-insensitively ([58d8f92](https://github.com/mogenius/renovate-operator/commit/58d8f92fe803781ecfe1da924d2f56383809e442))
* deacviate all trigger buttons if job hasnt been accepted by policy ([4d40c07](https://github.com/mogenius/renovate-operator/commit/4d40c07f03c07c3d6e34db8c00f35f3d9980c088))
* **deps:** update aws-sdk-go-v2 monorepo ([4966344](https://github.com/mogenius/renovate-operator/commit/4966344c31ceb263d1a52b4ea9904eecb09067b1))
* **deps:** update go module directive to v1.26.6 ([1107806](https://github.com/mogenius/renovate-operator/commit/11078061538797271416377d2331012f31bee8af))
* **deps:** update golang docker tag to v1.26.6 ([f52ac7c](https://github.com/mogenius/renovate-operator/commit/f52ac7c1806e2461c34713b4811bfa041cb40ab2))
* **deps:** update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.107.1 ([ad4c84b](https://github.com/mogenius/renovate-operator/commit/ad4c84b332592245d0e4421dda7e867fa1929c92))
* **deps:** update opentelemetry to v1.45.0 ([f497c81](https://github.com/mogenius/renovate-operator/commit/f497c8110d4096eda01c39a69441d4acb3dbc906))
* **deps:** update opentelemetry-go-contrib monorepo ([68cf1d2](https://github.com/mogenius/renovate-operator/commit/68cf1d24141e9d974639be4b248a636329753522))
* **metrics:** do not count approval pending PRs towards metrics ([863191c](https://github.com/mogenius/renovate-operator/commit/863191ceca06544753a86a76f4bb7f1acea16f6e)), closes [#584](https://github.com/mogenius/renovate-operator/issues/584)
* **parser:** do not add branches where no pr has been created yet ([a319bbe](https://github.com/mogenius/renovate-operator/commit/a319bbe767fc95dd1c4c3b9c36494196ab38d27d)), closes [#584](https://github.com/mogenius/renovate-operator/issues/584)
* **ui:** adapt the height of various objects to be more compact and coherent ([8b25b93](https://github.com/mogenius/renovate-operator/commit/8b25b93b5dc3df2a697cf96087a607c577d445fe)), closes [#559](https://github.com/mogenius/renovate-operator/issues/559)
* **ui:** add autoscroll and skip to top/bottom buttons in logs ([dafc88f](https://github.com/mogenius/renovate-operator/commit/dafc88f5dfbac79b62f0c199d5639120086e115b))


### Code Refactoring

* drop legacy job labels ([619e474](https://github.com/mogenius/renovate-operator/commit/619e474bd9603cc92dbbaa4533e7214486dd0b37))

## [5.6.0](https://github.com/mogenius/renovate-operator/compare/5.5.0...5.6.0) (2026-08-06)


### Features

* **chart:** add optional ListenerSet support to Helm chart ([50a8481](https://github.com/mogenius/renovate-operator/commit/50a84813d145fcbf0996215e2eb70562e64fb173))
* **parser:** enable report type logging by default and add parsing capabilities ([db56188](https://github.com/mogenius/renovate-operator/commit/db56188de993bd01e3a794393427b4b54cf9026a)), closes [#571](https://github.com/mogenius/renovate-operator/issues/571)
* **ui:** remember expansion of job cards([#547](https://github.com/mogenius/renovate-operator/issues/547)) ([2c72d82](https://github.com/mogenius/renovate-operator/commit/2c72d82738e312aa79f435f2f201688fedaba1d1))


### Bug Fixes

* **deps:** update aws-sdk-go-v2 monorepo ([c2ca29e](https://github.com/mogenius/renovate-operator/commit/c2ca29ea7114006c633b122eed43541d2ccc0342))
* **deps:** update aws-sdk-go-v2 monorepo ([a9eeffd](https://github.com/mogenius/renovate-operator/commit/a9eeffd5b2fa17f4f530bf9ec240ae93b191b4d4))
* **deps:** update aws-sdk-go-v2 monorepo ([c1da762](https://github.com/mogenius/renovate-operator/commit/c1da7626ac3e43511ec3400856ec93d02e56dc8c))
* **deps:** update aws-sdk-go-v2 monorepo ([1df1927](https://github.com/mogenius/renovate-operator/commit/1df1927273e2ccbea3f2d31dfeb708cbfcaee6e8))
* **deps:** update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.106.4 ([b7fbe44](https://github.com/mogenius/renovate-operator/commit/b7fbe449e6b24b04eb92c33055591915f5087958))
* **deps:** update module github.com/netresearch/go-cron to v0.15.1 ([10275a9](https://github.com/mogenius/renovate-operator/commit/10275a9cecab4b3b7826a17740f6af9ee71eb3f9))
* **deps:** update node.js to v24.18.1 ([504be2e](https://github.com/mogenius/renovate-operator/commit/504be2e181cbd0fa38782d41a701eeb843c67418))
* **deps:** update node.js to v24.19.0 ([8611259](https://github.com/mogenius/renovate-operator/commit/86112596ab252a00ea34c7f02df5312a54defd08))
* isolate executor jobs from autodiscovery ([cec8f37](https://github.com/mogenius/renovate-operator/commit/cec8f37e09fa1f7261e474e8f09d122cb3ce7775)), closes [#498](https://github.com/mogenius/renovate-operator/issues/498)
* **rbac:** narrow down secrets access ([d426bc8](https://github.com/mogenius/renovate-operator/commit/d426bc824ddf12a90068a413f1a2af01fe116a6f))
* **ui:** add Bitbucket Server pull request links ([ff7f745](https://github.com/mogenius/renovate-operator/commit/ff7f74580576e4c8c426522382cd557cb1909910))
* **ui:** move expand and collapse buttons on the same line as the search bar ([3c18b75](https://github.com/mogenius/renovate-operator/commit/3c18b75f3b962e80bb47fe42e33c77366c6f98f7))

## [5.5.0](https://github.com/mogenius/renovate-operator/compare/5.4.0...5.5.0) (2026-07-28)


### Features

* add priority class attribute support for pods ([1fc932a](https://github.com/mogenius/renovate-operator/commit/1fc932ab6b103808a2930f431a5740a1ee9971b0))
* **helm:** add values schema ([28d8f88](https://github.com/mogenius/renovate-operator/commit/28d8f881234515dc2ae88a851bb4c249394b2a40))
* **operator:** allow a per-job webhook base URL ([907bbcf](https://github.com/mogenius/renovate-operator/commit/907bbcf85be360ba1e4bcb94e0f6b073917b519f))
* **operator:** validate the format of spec.webhook.baseUrl ([4d11cdd](https://github.com/mogenius/renovate-operator/commit/4d11cdd99833608286f5ae3755012eb13fe951f5))
* **ui:** adding client side project search bar ([7602e4c](https://github.com/mogenius/renovate-operator/commit/7602e4c5798f2132ce582a999f399dd9bf967470)), closes [#537](https://github.com/mogenius/renovate-operator/issues/537)


### Bug Fixes

* **deps:** update kubernetes monorepo to v0.36.3 ([46acd8f](https://github.com/mogenius/renovate-operator/commit/46acd8f1f153f8815682fbb9a95d76f867f451ad))
* **deps:** update module github.com/prometheus/client_golang to v1.24.1 ([0386cef](https://github.com/mogenius/renovate-operator/commit/0386cef00e197dcd9fc469fa481bc71b0a788d3d))
* **deps:** update registry.k8s.io/kubectl docker tag to v1.36.3 ([0762855](https://github.com/mogenius/renovate-operator/commit/07628553761c4c96d847815d2442555594581b0b))
* **operator:** match synced webhooks on job identity, not full URL ([adf5bf5](https://github.com/mogenius/renovate-operator/commit/adf5bf598244d9420cd0820d774851c5a8498dd7))
* ran go fix against the codebase ([7b7fea9](https://github.com/mogenius/renovate-operator/commit/7b7fea9646377bd2096e5410c3b85d66faf31e53))
* **s3:** add missing trailing slash to pathname [#542](https://github.com/mogenius/renovate-operator/issues/542) ([fe99fe3](https://github.com/mogenius/renovate-operator/commit/fe99fe3cd8281549244e782c01a680e2ea100a9e))

## [5.4.0](https://github.com/mogenius/renovate-operator/compare/5.3.0...5.4.0) (2026-07-23)


### Features

* adding public endpoint option ([95b600a](https://github.com/mogenius/renovate-operator/commit/95b600adaef539443407c2bf66f2233204087a15)), closes [#502](https://github.com/mogenius/renovate-operator/issues/502)
* **helm:** Allow overriding the webhook baseUrl ([8e69df7](https://github.com/mogenius/renovate-operator/commit/8e69df78acd47998d032ca95a3f2249ef48011c9))
* hydrate metrics from crd state on startup ([6ba5a20](https://github.com/mogenius/renovate-operator/commit/6ba5a204b1566a61e46f25ac54c5b39359165b7c))
* improve grafana dashboard with new metrics ([cc454dd](https://github.com/mogenius/renovate-operator/commit/cc454dde42ded2dd01e0dd7a9fd0254578504e1a))
* **metrics:** expand Prometheus/OTel metrics for SRE and SecOps ([d58b98f](https://github.com/mogenius/renovate-operator/commit/d58b98fe9be0af9f265053957fe9e43274ecff3c))
* unify last Run and scheduled at into lastTransition ([2401ed4](https://github.com/mogenius/renovate-operator/commit/2401ed4793e51c4ce00f06003acaa48513832af5))


### Bug Fixes

* add metric hydration for missing metrics ([0f58f9c](https://github.com/mogenius/renovate-operator/commit/0f58f9cee90ebca6bbf859e270afc25c617da7be))
* adding missing metrics for gitea and bitbucket webhooks ([ecd9d9c](https://github.com/mogenius/renovate-operator/commit/ecd9d9c29ac7952fcb4687eac95f3f1d439d938b))
* **deps:** update aws-sdk-go-v2 monorepo ([7a640e6](https://github.com/mogenius/renovate-operator/commit/7a640e606b0a1fd230f05ff7dcc254f08191a410))
* **deps:** update dependency react to v19.2.8 ([d8db772](https://github.com/mogenius/renovate-operator/commit/d8db772932b7b6aab06b55d61bb4c902cd350653))
* **deps:** update dependency react-dom to v19.2.8 ([6b576ba](https://github.com/mogenius/renovate-operator/commit/6b576ba2d3954f7ab1c7dac5c54327a518527806))
* **deps:** update helm release valkey to v0.11.0 ([1813e0a](https://github.com/mogenius/renovate-operator/commit/1813e0a5685a54bc42d3e35fbf801d4b4960d624))
* **deps:** update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.106.0 ([d8af1d5](https://github.com/mogenius/renovate-operator/commit/d8af1d5c83d5274e3a493ad453d79df8f1307fe5))
* **otel:** upgrade semconv to match otel sdk version ([579a29a](https://github.com/mogenius/renovate-operator/commit/579a29a7ac90a6052ba4ef3b146684c8cd903f32))

## [5.3.0](https://github.com/mogenius/renovate-operator/compare/5.2.0...5.3.0) (2026-07-21)


### Features

* add extra labels to renovate job pods from global label templates ([8506a24](https://github.com/mogenius/renovate-operator/commit/8506a240b746d2b9d87962c431f30be4abc0cb78)), closes [#457](https://github.com/mogenius/renovate-operator/issues/457)
* adding a button to download the selected logs in the ui ([6552d39](https://github.com/mogenius/renovate-operator/commit/6552d3977bfbb4a4c50f2c6bbc7d562f0e782f82)), closes [#508](https://github.com/mogenius/renovate-operator/issues/508)
* adding the option for the cron parser to support jenkins style hash parsers ([3685505](https://github.com/mogenius/renovate-operator/commit/3685505fa190b387680769d48de87120a2302a0f)), closes [#495](https://github.com/mogenius/renovate-operator/issues/495)
* **ui:** persist log level filter ([e63e8ab](https://github.com/mogenius/renovate-operator/commit/e63e8ab7f99c3337a72588ada96b6508efc2853f))


### Bug Fixes

* adding printer columns to renovatejob crd ([0cc97c1](https://github.com/mogenius/renovate-operator/commit/0cc97c1deae6ca9e650860f63b6d81d261920c30))
* **deps:** update aws-sdk-go-v2 monorepo ([4be0aaf](https://github.com/mogenius/renovate-operator/commit/4be0aafc1ff63135e39115cc18da63f3a1a0ef9c))
* **deps:** update module github.com/aws/aws-sdk-go-v2/service/s3 to v1.105.2 ([998ff8c](https://github.com/mogenius/renovate-operator/commit/998ff8ce59b8f8ff4b55acae89442377c1d4b26d))
* **deps:** update module github.com/go-logr/logr to v1.4.4 ([a50454e](https://github.com/mogenius/renovate-operator/commit/a50454e8b09aebf9bd1bfb7c0b52c29ea66ad2b8))
* **deps:** update module github.com/prometheus/client_golang to v1.24.0 ([5f79fb2](https://github.com/mogenius/renovate-operator/commit/5f79fb2692e8d4fa12e0b8035a5623002ae4397c))
* honor group settings when authorizing users ([da04964](https://github.com/mogenius/renovate-operator/commit/da04964c9030d6e3ad10801756903aada3cb6103))

## [5.2.0](https://github.com/mogenius/renovate-operator/compare/5.1.0...5.2.0) (2026-07-10)


### Features

* **helm:** add externalKeyValueStore values for external connections ([09e2cc3](https://github.com/mogenius/renovate-operator/commit/09e2cc31ccb636568a08ebe8990a1bb7b8a7c6c4))
* **operator:** support username and TLS for host-based Valkey config ([869635a](https://github.com/mogenius/renovate-operator/commit/869635a3a06e267e04de0fa74ab0e9630b02a22e))


### Bug Fixes

* correctly escape spaces in user password when building the valkey url ([93c47c7](https://github.com/mogenius/renovate-operator/commit/93c47c768e218b44ccd96b8fe9f7bf857ba248df))
* **helm:** grafana dashboard label selector allow default overwrite ([1655691](https://github.com/mogenius/renovate-operator/commit/16556913fdc1ed7ebd01a30b17fc465bd11bb1ad)), closes [#485](https://github.com/mogenius/renovate-operator/issues/485)
* **operator:** surface the underlying error when KV store init fails ([9f91cf0](https://github.com/mogenius/renovate-operator/commit/9f91cf058a821702e1b0f6efafe214141ea85254))
* **ui:** do not close log expansion if logs are selected ([21da12f](https://github.com/mogenius/renovate-operator/commit/21da12f7329e61d22cd9329a27803f5ff78a0925)), closes [#491](https://github.com/mogenius/renovate-operator/issues/491)
* **webhook:** Trigger Renovate on BitBucket PR merge ([d88d8b0](https://github.com/mogenius/renovate-operator/commit/d88d8b0cf2864b55369131ef78c88572ea31d4fe))
* **webhook:** Trigger Renovate on Forgejo PR merge ([71c1097](https://github.com/mogenius/renovate-operator/commit/71c1097259afc75cac27359fa3b388e2888532c3))
* **webhook:** Trigger Renovate on Gitea PR merge ([4935563](https://github.com/mogenius/renovate-operator/commit/4935563f6ccfd22f65faa58890d6fd31c1aecc61))
* **webhook:** Trigger Renovate on Github PR merge ([02a522c](https://github.com/mogenius/renovate-operator/commit/02a522c5701e9ae0a0214f17362799ecf4cc3414))
* **webhook:** Trigger Renovate on Gitlab MR merge ([e8000cb](https://github.com/mogenius/renovate-operator/commit/e8000cb65825084426f4261423b515d93d4d825d)), closes [#463](https://github.com/mogenius/renovate-operator/issues/463)

## [5.1.0](https://github.com/mogenius/renovate-operator/compare/5.0.1...5.1.0) (2026-07-09)


### Features

* **ui:** make the OIDC groups claim name configurable ([9359d11](https://github.com/mogenius/renovate-operator/commit/9359d110f868f5f73dee50ef65fcf0c42f012533))


### Bug Fixes

* **deps:** update aws-sdk-go-v2 monorepo ([98eed90](https://github.com/mogenius/renovate-operator/commit/98eed90a6d96646e9d926c1a1c9e8f7696e8f391))
* **deps:** update module github.com/coreos/go-oidc/v3 to v3.20.0 ([52e8119](https://github.com/mogenius/renovate-operator/commit/52e81198b99fdd874a7396ddf863e3b69874f825))
* **ui:** avoid polling missing discovery jobs ([657dbd2](https://github.com/mogenius/renovate-operator/commit/657dbd206df0853b39357ae9aadd75b856f5fba7))

## [5.0.1](https://github.com/mogenius/renovate-operator/compare/5.0.0...5.0.1) (2026-07-08)


### Bug Fixes

* automatic webhook sync for forgejo/gitea does not properly set authorization header if configured ([94acbfb](https://github.com/mogenius/renovate-operator/commit/94acbfbce17328147cbbdc9d443d92e65e14143d)), closes [#476](https://github.com/mogenius/renovate-operator/issues/476)

## [5.0.0](https://github.com/mogenius/renovate-operator/compare/4.14.1...5.0.0) (2026-07-08)


### ⚠ BREAKING CHANGES

* During the development of this feature the existing forgejo webhook sync expirienced a major rewrite. Forgejo users please note the updated docs and plan accordingly

### Features

* add automatic webhook sync for all suported git providers ([90532a5](https://github.com/mogenius/renovate-operator/commit/90532a52d486129fd8a09999762c5c38dc96c1fe))
* add scheme override for auth redirect and webhook base URL ([#452](https://github.com/mogenius/renovate-operator/issues/452)) ([c5128f1](https://github.com/mogenius/renovate-operator/commit/c5128f1a48ad7fe4c70226ab34e9be83e7ab1673))
* adding s3 configuration for renovate job logs and caching ([1010817](https://github.com/mogenius/renovate-operator/commit/1010817197fd15aa793fd1249d985937635bf7a9)), closes [#329](https://github.com/mogenius/renovate-operator/issues/329)
* allow setting the webhook host to the ui host for small deployments ([738cd2f](https://github.com/mogenius/renovate-operator/commit/738cd2f6e5954904e417c56966a9b0eb7737fd79)), closes [#460](https://github.com/mogenius/renovate-operator/issues/460)
* **operator:** serve UI, API and auth under a configurable sub-path ([58787c2](https://github.com/mogenius/renovate-operator/commit/58787c264d6629c127be65742881396ef36a1a75))
* **webhook:** support Standard Webhooks signature authentication ([#454](https://github.com/mogenius/renovate-operator/issues/454)) ([dd01c86](https://github.com/mogenius/renovate-operator/commit/dd01c865ae1bb039358d1c81313a54e51fe20b0b))


### Bug Fixes

* **deps:** update go module directive to v1.26.5 ([12bebd7](https://github.com/mogenius/renovate-operator/commit/12bebd7edecad8cba3a8286fd9cde3ee55f37a44))
* **deps:** update golang docker tag to v1.26.5 ([afc9d80](https://github.com/mogenius/renovate-operator/commit/afc9d802f0e1ac70f41a7231e24db318e5326778))

## [4.14.1](https://github.com/mogenius/renovate-operator/compare/4.14.0...4.14.1) (2026-07-02)


### Bug Fixes

* **helm:** trim service account name helper ([4f1c0a4](https://github.com/mogenius/renovate-operator/commit/4f1c0a4d2b44a8696708a09b307fc273e4d9c02b))
* **operator:** propagate operator labels to pod templates so pods carry them for NetworkPolicies ([89bff26](https://github.com/mogenius/renovate-operator/commit/89bff26a2185f4b60e873a55b5287980f90379bf))
* **ui:** serve /components/ assets without auth to prevent blank page ([699d755](https://github.com/mogenius/renovate-operator/commit/699d755f0bc5ed98ee516680a0ceb2e78bac362d))

## [4.14.0](https://github.com/mogenius/renovate-operator/compare/4.13.0...4.14.0) (2026-06-25)


### Features

* **helm:** allows adding labels to service monitor ([b3596f1](https://github.com/mogenius/renovate-operator/commit/b3596f140e4a181d9fee087ed1ea81749eeea308))


### Bug Fixes

* **deps:** update node.js to v24.18.0 ([f5ec451](https://github.com/mogenius/renovate-operator/commit/f5ec451402f2d1a7ac5d9d3d82ee0d946dad45f9))
* **forgejo:** address review — drain 404 body, assert DELETE in test ([5684c03](https://github.com/mogenius/renovate-operator/commit/5684c038a52010c9f6a0f4c5e23ccd0d8428c8f6))
* **forgejo:** treat 404 as success when deleting a webhook ([730b30f](https://github.com/mogenius/renovate-operator/commit/730b30f755231af0afc6dbab5bdbf1d21d46ca72))
* **webhook-sync:** address review — real 403 skip, preallocation, log wording ([95705e3](https://github.com/mogenius/renovate-operator/commit/95705e3c96c6b5d73000910ef2e7397279c74da8))
* **webhook-sync:** sync webhooks for autodiscovered repos without a topic ([0a52abf](https://github.com/mogenius/renovate-operator/commit/0a52abfed694a64de88fca6b978d4921585ab98b))

## [4.13.0](https://github.com/mogenius/renovate-operator/compare/4.12.4...4.13.0) (2026-06-22)


### Features

* adding annotation based trigger for job and discovery triggers ([d3b01a3](https://github.com/mogenius/renovate-operator/commit/d3b01a38a54ebea75c139e2f152aae475419bb84)), closes [#413](https://github.com/mogenius/renovate-operator/issues/413)
* **build:** enable image signing ([4901ca0](https://github.com/mogenius/renovate-operator/commit/4901ca065aa778352fef33365510666aa76f239c))
* stream logs of running jobs ([fae694b](https://github.com/mogenius/renovate-operator/commit/fae694be8bbc0fce3a079b5a656f5dbd42d2f19a)), closes [#427](https://github.com/mogenius/renovate-operator/issues/427)
* try best effort matching for webhooks without job or namespace ([29c31f1](https://github.com/mogenius/renovate-operator/commit/29c31f1c0d0bb5f5128ad94276a0008df8f73bad))


### Bug Fixes

* **deps:** update helm release valkey to v0.10.0 ([dfac012](https://github.com/mogenius/renovate-operator/commit/dfac01289e026b642c0d6af2e78b0f43554dc63c))
* **deps:** update module github.com/coreos/go-oidc/v3 to v3.19.0 ([fd031e7](https://github.com/mogenius/renovate-operator/commit/fd031e7c1a9ad8de94dd294b20377b531faf5365))
* **deps:** update module github.com/valkey-io/valkey-go to v1.0.76 ([d47aeac](https://github.com/mogenius/renovate-operator/commit/d47aeaca879f9ea6383a1c7359872126be15e2cb))
* **deps:** update node.js to v24 ([a20b5ca](https://github.com/mogenius/renovate-operator/commit/a20b5cac9aaa7e026fe944bc30a12ee11dccecad))
* **deps:** update node.js to v24.17.0 ([9374b3b](https://github.com/mogenius/renovate-operator/commit/9374b3be885cbab6758b91c583cffacf8a20b912))
* **deps:** update react monorepo to v19 ([d0596d9](https://github.com/mogenius/renovate-operator/commit/d0596d97d3ab946570335fa38606e41c8f36a558))
* **jobs:** trim project label selector if it is too long ([d824e76](https://github.com/mogenius/renovate-operator/commit/d824e76a122861e72918df9c7426cd32a8f1e790)), closes [#436](https://github.com/mogenius/renovate-operator/issues/436)
* **rbac:** allow operator in namespace only mode to patch jobs ([9ef0c22](https://github.com/mogenius/renovate-operator/commit/9ef0c22ea690c91a9fe0b18a179871955ae0f647)), closes [#429](https://github.com/mogenius/renovate-operator/issues/429)
* remove redundant api call on renovate status updates ([432f84f](https://github.com/mogenius/renovate-operator/commit/432f84ffe9515cc1a6bce67ade86afefa775ccc3))
* **ui:** add badge to show if logs are streaming or complete ([47dad6b](https://github.com/mogenius/renovate-operator/commit/47dad6b8d4826898254c5a833f0c9d0bb534e7c9))
* **ui:** react 19 migration ([e4a2f53](https://github.com/mogenius/renovate-operator/commit/e4a2f535e886a70efee074e9c0545163f9d177ca))
* **ui:** set Cache-Control headers on static assets ([#433](https://github.com/mogenius/renovate-operator/issues/433)) ([47772d3](https://github.com/mogenius/renovate-operator/commit/47772d3b906f957d40d6f1b2234bce30622a3ea2))

## [4.12.4](https://github.com/mogenius/renovate-operator/compare/4.12.3...4.12.4) (2026-06-17)


### Bug Fixes

* **ui:** pin all js dependencies to a fixxed version and add renovate manager ([ddf4188](https://github.com/mogenius/renovate-operator/commit/ddf41880d61ac854859033ebc0fe85d25013cda1))

## [4.12.3](https://github.com/mogenius/renovate-operator/compare/4.12.2...4.12.3) (2026-06-17)


### Bug Fixes

* **ui:** downgrade to babel version 7 ([3bd78b7](https://github.com/mogenius/renovate-operator/commit/3bd78b7207ea5c6f2e2a4daba4d70c035f6258ec)), closes [#408](https://github.com/mogenius/renovate-operator/issues/408)

## [4.12.2](https://github.com/mogenius/renovate-operator/compare/4.12.1...4.12.2) (2026-06-17)


### Bug Fixes

* **helm:** add missing patch permission for jobs resources ([f26c522](https://github.com/mogenius/renovate-operator/commit/f26c52243ac083081ec5145a37859b719229532e))

## [4.12.1](https://github.com/mogenius/renovate-operator/compare/4.12.0...4.12.1) (2026-06-16)


### Bug Fixes

* **helm:** allow disabeling pkce using helm values ([b2d07ec](https://github.com/mogenius/renovate-operator/commit/b2d07ec3975b9b6504229112c8a3d8df7135fef9))

## [4.12.0](https://github.com/mogenius/renovate-operator/compare/4.11.0...4.12.0) (2026-06-16)


### Features

* enable pkce auth flow ([efdbe60](https://github.com/mogenius/renovate-operator/commit/efdbe60090c281c5fa7a3e416945d9650013431c)), closes [#186](https://github.com/mogenius/renovate-operator/issues/186)
* **ui:** reflect selected dashboard filter in URL ([fc279a4](https://github.com/mogenius/renovate-operator/commit/fc279a4dfb7c3125d2af547ed80f2306f73a3276))


### Bug Fixes

* **deps:** update go module directive to v1.26.4 ([f3b3e35](https://github.com/mogenius/renovate-operator/commit/f3b3e358a786434b197747fdd380e5c5f04bd5f3))
* **deps:** update kubernetes monorepo to v0.36.2 ([56873a1](https://github.com/mogenius/renovate-operator/commit/56873a1b9c86cd153adadbe0b967400a8404adaf))
* **deps:** update registry.k8s.io/kubectl docker tag to v1.36.2 ([b6a47e0](https://github.com/mogenius/renovate-operator/commit/b6a47e0f35a5e86745b7f2ca484dbf1657c6c231))
* **discovery:** check for discovery job status within the lock to mitigate duplicated discovery-jobs ([3384743](https://github.com/mogenius/renovate-operator/commit/3384743b76be1de35115273d8afdb8d125f81a33))
* **executor:** adding early exit if parallelization limit is already reached ([3d8f191](https://github.com/mogenius/renovate-operator/commit/3d8f1916011630c9501c2cd5eb0ea53700d285e4))
* **executor:** improve loop performance in identifying next project to run ([f80d66f](https://github.com/mogenius/renovate-operator/commit/f80d66fde117ca0ef5b756213cd8c97f49a46c7b))
* **executor:** reduce duplicated api calls by only running ensure redis once per namespace ([dfd8f33](https://github.com/mogenius/renovate-operator/commit/dfd8f332fad3ad2cf807738c3bea3f9667ea65e1))
* return sensible error message if a non existing project is being updated ([253d258](https://github.com/mogenius/renovate-operator/commit/253d258eded97ab589f938f153080baebebe43ce)), closes [#383](https://github.com/mogenius/renovate-operator/issues/383)
* **ui:** place log level badges next to each other ([b32430a](https://github.com/mogenius/renovate-operator/commit/b32430a1d8b1f4cd24457366df34e798acce178d))

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
