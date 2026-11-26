# Upgrade Guide

## Unreleased

## 0.4.0

Nothing to change: the API of 0.3.2 is intact and a client that does not set `Config.Kokoro` is the client it was. The keys of Piper audio are the ones 0.3.2 made.

- To add the female Brazilian voice, install the Kokoro runtime and model in the service image (`docs/Dockerfile.kokoro.snippet`: the packages of `docs/kokoro-requirements.txt` by `pip --target`, `kokoro-v1.0.onnx` and `voices-v1.0.bin`, checked by sha256, and `docs/kokoro-speak.py` with the interpreter on its first line), and set `Config.Kokoro` with the three paths and `Env: PYTHONPATH=/opt/kokoro`. The service's configuration then serves `pf_dora`, `pm_alex` and `pm_santa` beside the Piper voices; give `DefaultVoice` the voice you want when none is named.
- A node of a service that will not serve Kokoro needs nothing, but an application whose `Remote` calls it for a Kokoro voice gets `ErrUnknownVoice` once every node has refused it. Call `Remote.Voices(ctx)` at boot to see which node serves what.
- `Remote` now asks the next node when one answers `unknown_voice` (see the changelog). Code that relied on the first refusal being final sees the same `errors.Is(err, piper.ErrUnknownVoice)`, after one request to each node.
- A `Handler` answers `GET` with a valid token with the list of voices. Nothing calls it that did not before; a proxy that forbids `GET` on the path only loses `Remote.Voices`, and the calls themselves still work.

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
