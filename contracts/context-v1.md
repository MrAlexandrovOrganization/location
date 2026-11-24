# Measurement context v1

Owner: backend Location. The optional `context` object accompanies an immutable
measurement; Android raw schema_version remains 2. The legacy Android/iOS JSON
routes also accept `context` within their existing 4096-byte body limit. Raw
measurements keep their 65536-byte body limit. Context normalization is limited
to 8192 bytes. Unknown versions, invalid types/values and oversized context are
retained in raw_payload with a diagnostic status, without rejecting a valid point
or influencing hidden/quality_reason/blacklist/geofence policy.

The [example](examples/context-v1.json) is a complete Android observation. On iOS,
use client.platform=ios and, if needed, platform.ios for native attributes. No
Android attributes are required. All blocks except integer version=1 are optional.
Unknown fields are preserved in original raw bytes; the normalized projection
contains recognized common fields and the native platform object.

- client: platform (must agree with authenticated route when supplied),
  installation_id (UUID), app_version (up to 100 bytes). These are observations,
  not authorization. device_id is server-assigned from the authenticated credential
  slot: mobile:android:primary or mobile:ios:primary. Current credentials support
  one tracker per platform. Multiple phones of the same platform require distinct
  credential bindings before their histories can safely be used for filtering.
  No cross-device filter changes are part of v1.
- steps: status, and, when observed, nonnegative integer count with RFC3339 UTC
  from/to defining the exact observation interval. count=0 is a genuine observed
  zero; never substitute zero for missing permission or unavailable readings.
  Android counter snapshots describe the interval between two counter events,
  not necessarily the interval between location fixes. Counter resets, restarts
  and missing baselines begin a new interval. These are point snapshots, not a
  lossless journal of every step. Existing point compaction remains independent.
- movement: status, optional last_event_at and event (nonempty, up to 64 bytes),
  optional observing_since. Event and last_event_at must occur together. Initial
  event names: significant_motion, step. They indicate events, not a transport
  classifier and never prove stillness through absence. Do not label significant
  motion as walking. Future activity flags may coexist (e.g. iOS automotive and
  stationary); v1 does not require a mutually exclusive activity enum.
- collection: optional mode=normal/stationary/unknown, optional integer
  requested_interval_ms in [0,86400000]. This is an app policy hint, not a promise
  of actual sampling frequency. Clients without an interval omit it.
- platform.android: delivery_channel=active/passive; passive_reason is the
  collector decision (waiting, disabled, unavailable, screen_off, mock,
  invalid_coordinates, invalid_accuracy, coarse, stale, future, duplicate,
  expired, using_passive, throttled); step_detector_status/step_counter_status
  report availability/registration. Native fields are optional and extensible.

Observation status values: observed, unsupported, permission_denied, disabled,
unavailable, unknown. Non-observed steps omit count/from/to. Timestamp years must
be 2000–2100 and from<=to. Freshness is derived from observation times, not arrival
at the server; Grafana exposes age relative to the location's recorded_at. Negative
ages reveal observations later than the fix and must not be silently treated as
fresh evidence. Android uses elapsed clocks within the same boot/session to select
events at/before the fix, converting to UTC using the callback's wall/elapsed pair.
No old queued measurement is enriched at upload time. A retry keeps UUID and raw
bytes, including context. Cross-subscription dedup compares only the underlying
system measurement, excluding collection-time context.

Storage: location_measurement_context is inserted in the same transaction as the
point. location_context provides the read-only typed projection and raw context.
Malformed JSON envelopes still fail normal input validation; optional context
errors never turn an otherwise valid coordinate into a hidden point. The backend
must be deployed with migration 013 before enabling context on clients.
