# Upgrade Guide

## Unreleased

## 0.3.2

Nothing to change.

## 0.3.1

Nothing to change. `Remote` now waits up to `BusyWait` (5 seconds) when every node is busy, where it used to fail at once; set `BusyWait: -1` to keep the old behaviour. If you limited Piper's threads with `OMP_NUM_THREADS`, it never took effect: see `docs/sitecustomize.py`.

## 0.3.0

Nothing to change: every addition is optional and the API of 0.2.0 is intact.

- To run speech as a service, build a `*Client` with `Format: piper.FormatOpus` and mount `piper.NewHandler` where the service listens.
- To use a service from an application, build `piper.NewRemote` and pass it where the application passed the `*Client` (a `piper.Speaker`; it is also `piper.Keyed`, so `piper.NewCache` takes it). Its `Key` is not the `*Client`'s: the keys of audio stored by a cache over a local client do not match the remote's, so the first calls after switching make the audio again.
- The manifest now declares `network = true`. An application whose own manifest or policy forbids network use by its dependencies should be told.

## 0.2.0

Nothing to change: `NoiseScale` and `NoiseW` are new fields whose zero value is the old behaviour, and the keys of a client that does not set them are the ones 0.1.0 made.

## 0.1.0

The first release: nothing to upgrade from. Install it as the README says.
