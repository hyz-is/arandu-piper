# Security policy

## Supported versions

The latest minor release of Piper receives security fixes. Older minor
releases do not: a published Go module version is immutable, so a fix is a new
release rather than a moved tag.

## Reporting a vulnerability

Report it privately, through GitHub's advisory form:

<https://github.com/hyz-is/arandu-piper/security/advisories/new>

Do not open a public issue and do not describe the problem in a pull request.
A report that arrives in public is a report every reader of this repository can
act on before there is a release to upgrade to.

Include what you need to reproduce it: the version, the wiring, and the request
or call that triggers it.

Expect an acknowledgement within a few days. If the report is confirmed, the
fix, the release and the advisory are published together, and you are credited
unless you ask not to be.

## What is in scope

Anything that lets a caller do more than make speech of the text it handed
over. In particular:

- a way to make the package run a program, or pass an option, other than the
  ones `Config` names, for example through the text;
- a path from the text, or from the program's output, to a file outside the
  call's private temporary directory;
- a working directory, an audio file or a text that survives the call, or
  appears in an error;
- the application's environment (its secrets) reaching Piper or ffmpeg;
- a call that outlives its context or timeout, or leaves a running process
  behind.

## What is not

- A vulnerability in Piper, onnxruntime, espeak-ng, ffmpeg or a voice. Report
  that to the project that owns it.
- A text that is simply slow for the hardware it runs on, within the limits the
  application configured. Choosing `MaxChars`, `Timeout` and how many calls
  run at once is the application's.
