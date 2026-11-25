"""Loaded by every Python process that has /opt/piper on its path, which is
Piper and nothing else in this image.

Two things, both about onnxruntime, which Piper runs on:

* Threads. onnxruntime starts one intra-op thread per core the machine shows
  (32 on these hosts), and the container is allowed two CPUs: the threads spin
  against the quota and a sentence of six seconds took eight to synthesize, where
  two threads take one and a half. OMP_NUM_THREADS does not reach it (this build
  is not OpenMP), and the piper program has no option for it, so the session
  options are set here, from PIPER_ORT_THREADS.
* Telemetry. onnxruntime carries a telemetry client (it logs "Failed to persist
  telemetry device ID"). Nothing the person said may leave the machine, so the
  events are turned off before any session exists.
"""
import os

try:
    import onnxruntime as _ort
except Exception:  # Piper will say what is missing; this must never be the error.
    _ort = None

if _ort is not None:
    try:
        _ort.disable_telemetry_events()
    except Exception:
        pass

    _threads = int(os.environ.get("PIPER_ORT_THREADS", "0") or 0)
    if _threads > 0:
        _Session = _ort.InferenceSession

        def _session(path_or_bytes, sess_options=None, *args, **kwargs):
            options = sess_options or _ort.SessionOptions()
            options.intra_op_num_threads = _threads
            options.inter_op_num_threads = 1
            return _Session(path_or_bytes, options, *args, **kwargs)

        _ort.InferenceSession = _session
