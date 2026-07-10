Feature: Publish Inster metrics to MQTT with Home Assistant autodiscovery

  The service authenticates to the (mock) Bluelink EU API, reads the Inster's
  CCS2 status, and republishes it to MQTT with HA autodiscovery.

  Background:
    Given a charging, plugged-in Inster on the Bluelink account

  Scenario: Authenticate and publish discovery on startup
    When the service starts up
    Then the battery sensor discovery config is published retained
    And the charging binary_sensor discovery config is published retained
    And the device_tracker discovery config is published retained
    And availability "online" is published retained

  Scenario: Publish battery and charging state from a cached poll
    When the service starts up
    And a cached poll runs
    Then the state topic reports battery 62
    And the state topic reports charging true
    And the state topic reports plugged_in true
    And the readiness endpoint reports ready

  Scenario: Publish state for a CCS1 vehicle
    Given the Inster speaks the CCS1 protocol
    When the service starts up
    And a cached poll runs
    Then the state topic reports battery 62
    And the state topic reports charging true
    And the state topic reports plugged_in true
    And the readiness endpoint reports ready

  Scenario: Refresh the access token when it has expired
    Given a stored but expired access token with a valid refresh token
    When the service starts up
    And a cached poll runs
    Then the token endpoint received a "refresh_token" grant
    And no device registration occurred
    And the state topic reports battery 62

  Scenario: Daily force refresh uses the force endpoint
    When the service starts up
    And a scheduled force refresh runs
    Then the force status endpoint was called
    And the state topic reports battery 63

  Scenario: Force refresh is skipped when the car is unplugged
    Given the Inster is unplugged
    And force refresh is gated on being plugged in
    When the service starts up
    And a scheduled force refresh runs
    Then the force status endpoint was not called

  Scenario: Graceful degradation keeps last state and flips readiness
    When the service starts up
    And a cached poll runs
    And the Bluelink API starts failing
    And 3 cached polls run
    Then the readiness endpoint reports not ready
    And the state topic still reports battery 62

  Scenario: Graceful shutdown publishes offline
    When the service starts up
    And the service shuts down
    Then availability "offline" is published retained
