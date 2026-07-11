# Changelog

## [1.1.0](https://github.com/gsdevme/hyundai-bluelink-mqtt/compare/v1.0.1...v1.1.0) (2026-07-11)


### Features

* **homeassistant:** convert odometer to configured DISTANCE_UNIT ([c1f1801](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/c1f1801abb0147ff68b605f1b538fb48b5708d78))

## [1.0.1](https://github.com/gsdevme/hyundai-bluelink-mqtt/compare/v1.0.0...v1.0.1) (2026-07-11)


### Bug Fixes

* **homeassistant:** make Range unit configurable via DISTANCE_UNIT ([5cca545](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/5cca5459195368f99badaa6e533ec2d879dbc449))

## 1.0.0 (2026-07-11)


### Features

* **bluelink:** add a hidden dump command to capture raw API responses ([5b24190](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/5b24190e0aa2a7d7bb93932bed26055f7c8a4c48))
* **bluelink:** implement read-only Hyundai EU CCS2 client ([14d7c0e](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/14d7c0ed848c5930b33cc513464bedfe13379d89))
* **bluelink:** parse CCS1 vehicle status into VehicleState ([9508fae](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/9508faee8dda6f0b747b542ff20da44c7c6c6dc4))
* **cmd:** add a --ccs1 flag to the mock command ([1d1cfce](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/1d1cfce1238582861e99715c4bdc4ea7b7f00e02))
* **cmd:** add serve and mock subcommands with graceful shutdown ([101748e](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/101748ee8582517c82c2d71aaf3a2cd1e1c92dce))
* **cmd:** publish state for CCS1 vehicles ([9106d4c](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/9106d4c4e84ca930866c5d69d1d943e197acdb2d))
* **config,mqtt,tokens:** add config, MQTT client and kube token store ([29367e2](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/29367e2db2459d6ae2032fd6f3b7193bb2d53951))
* **config:** default POLL_INTERVAL to 45m ([5fa5188](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/5fa51885e7b522e92a31a92e92cf14017da52bf9))
* **config:** resolve Bluelink target via MODE switch ([93323b7](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/93323b71f4ce5e2cb38aecb46b25927aa4f52de6))
* **ha:** add Home Assistant discovery and MQTT publisher ([0368b1f](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/0368b1f6d45abe6d9644c47c2d185033ba053422))
* **mock:** add in-process Bluelink EU mock server ([7a9a281](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/7a9a28199b9bab5de5868d7df3e4dcc98316bd30))
* **mock:** serve CCS1 vehicle status endpoints ([78c7adf](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/78c7adf4347678e58332d40fb86485573cf1c94c))
* **scheduler,health:** add poll loop, daily force refresh and probes ([52ad22b](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/52ad22b824585675d2ddb229a1d0c03c75404b25))
* **server:** serve a / status page from a new internal/server package ([1e04ea6](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/1e04ea664f7e461d1e3233cf2f2f283fd5878ca3))
* **server:** show live non-personal metrics on status page ([0540909](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/0540909be73d1a344185dfe65f2543717b9907e3))


### Bug Fixes

* **bluelink:** handle the distinct CCS1 force status envelope ([f57230a](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/f57230ab4727412d6620f328958fc0a87e9d229e))
* **cmd:** fail fast when the serve health server cannot bind ([7657b41](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/7657b416583fc0e352083b1d71a36baca00cbd90))
* **cmd:** report errors to stderr and exit non-zero from main ([700e56c](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/700e56c9ada621255529cfe273efe2c91393afb8))
* **cmd:** shut down mock server gracefully on SIGTERM/SIGINT ([0c42bcb](https://github.com/gsdevme/hyundai-bluelink-mqtt/commit/0c42bcbdd7e75eef9f7268b8f5f832db4c7e4440))
