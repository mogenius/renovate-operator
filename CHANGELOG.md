# Changelog

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
