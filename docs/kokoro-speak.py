#!/usr/bin/env python3
"""kokoro-speak: the program the Kokoro voices of arandu-piper run.

It is the whole contract between the Go package and the Kokoro engine, and it
is deliberately small. The text is read from standard input (never from an
argument, so it does not show in a process list), the speech is written as a
mono 16-bit WAV to --output, and nothing else is written anywhere. Standard
output stays empty. A failure prints one short line to standard error and exits
with a status other than zero; the line never carries the text.

    kokoro-speak --model kokoro-v1.0.onnx --voices voices-v1.0.bin \
        --voice pf_dora --lang pt-br --threads 2 --output speech.wav < text.txt

Needs the packages of docs/kokoro-requirements.txt (kokoro-onnx, MIT, and what
it asks for: onnxruntime, numpy, espeak-ng through espeakng-loader). The first
line of this file is replaced by the path of the interpreter that has them
installed, as pip does for the scripts it writes (see docs/Dockerfile.kokoro.snippet).
"""
import argparse
import re
import sys
import wave

# What is allowed to put its message on standard error. Any other failure is
# reported by its class alone: an exception raised while the text was being
# read can quote it.
SAFE_TO_SHOW = (ImportError, FileNotFoundError, PermissionError)

LANG = re.compile(r"^[a-z]{2,3}(-[a-z0-9]{2,8})?$")
VOICE = re.compile(r"^[a-z]{2}_[a-z0-9]+$")


def main(argv):
    ap = argparse.ArgumentParser(prog="kokoro-speak", allow_abbrev=False)
    ap.add_argument("--model", required=True, help="the Kokoro ONNX model")
    ap.add_argument("--voices", required=True, help="the voices file (voices-v1.0.bin)")
    ap.add_argument("--voice", required=True, help="a voice of the file, such as pf_dora")
    ap.add_argument("--lang", required=True, help="the espeak-ng language, such as pt-br")
    ap.add_argument("--speed", type=float, default=1.0, help="1 is the voice's own pace; above 1 is faster")
    ap.add_argument("--threads", type=int, default=2, help="threads onnxruntime may use")
    ap.add_argument("--sentence-pause", type=float, default=None, help="seconds of silence after a sentence")
    ap.add_argument("--output", required=True, help="the WAV file to write")
    args = ap.parse_args(argv)

    if not VOICE.match(args.voice) or not LANG.match(args.lang):
        raise ValueError("voice or language is not well formed")
    if not 0.25 <= args.speed <= 4 or not 1 <= args.threads <= 64:
        raise ValueError("speed or threads out of range")

    text = " ".join(sys.stdin.read().split())
    if not text:
        raise ValueError("no text on standard input")

    import numpy as np
    import onnxruntime as ort
    from kokoro_onnx import Kokoro

    # Nothing the person said may leave the machine: no telemetry events.
    try:
        ort.disable_telemetry_events()
    except Exception:
        pass

    # onnxruntime starts a thread per core it sees unless it is told otherwise,
    # which under a CPU quota is slower than a few threads. Set here, from
    # --threads, not by an environment variable it does not read.
    options = ort.SessionOptions()
    options.intra_op_num_threads = args.threads
    options.inter_op_num_threads = 1
    session = ort.InferenceSession(args.model, sess_options=options, providers=["CPUExecutionProvider"])
    kokoro = Kokoro.from_session(session, args.voices)

    extra = {}
    if args.sentence_pause is not None:
        extra["sentence_pause"] = args.sentence_pause
    samples, rate = kokoro.create(text, voice=args.voice, speed=args.speed, lang=args.lang, **extra)
    if len(samples) == 0:
        raise ValueError("no audio was made")

    pcm = (np.clip(samples, -1.0, 1.0) * 32767.0).astype("<i2")
    with wave.open(args.output, "wb") as out:
        out.setnchannels(1)
        out.setsampwidth(2)
        out.setframerate(rate)
        out.writeframes(pcm.tobytes())


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except SystemExit:
        raise
    except SAFE_TO_SHOW as error:
        sys.stderr.write("kokoro-speak: %s: %s\n" % (type(error).__name__, error))
        sys.exit(1)
    except BaseException as error:
        sys.stderr.write("kokoro-speak: %s\n" % type(error).__name__)
        sys.exit(1)
