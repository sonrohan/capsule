# Android Capsule Sample

This sample is a minimal Android app used to demonstrate a committed, curated
Capsule execution snapshot.

Most `.capsule/` directories should not be committed. This one is intentionally
published because it is small sample evidence for the repo documentation.

## Published Capsule

Capsule ID:

```text
cap_23dd3469056c
```

Location:

```text
examples/android/.capsule/capsules/cap_23dd3469056c/
```

The Capsule was recorded against Git commit:

```text
39cb3caf9e993540234ff2b8f7a476afb536f172
```

## Commands Recorded

The session ran these commands from `examples/android`:

```sh
../../capsule start
../../capsule run ./gradlew :app:assembleDebug
../../capsule run ./gradlew :app:testDebugUnitTest
../../capsule run ./gradlew :app:lintDebug
../../capsule finish
```

All three Gradle commands exited successfully:

```text
1. ./gradlew :app:assembleDebug       exit=0
2. ./gradlew :app:testDebugUnitTest   exit=0
3. ./gradlew :app:lintDebug           exit=0
```

## What Capsule Captured

The published snapshot includes:

```text
manifest.json
commands.json
metadata.json
logs/
artifacts/
```

The artifacts prove that each command produced or verified the expected Android
outputs:

```text
artifacts/001-app__build__outputs__apk__debug__app-debug.apk
artifacts/002-app__build__test-results__testDebugUnitTest__TEST-app.capsule.ExampleUnitTest.xml
artifacts/003-app__build__reports__lint-results-debug.html
artifacts/003-app__build__reports__lint-results-debug.xml
```

The logs are stored separately per command:

```text
logs/001-combined.log
logs/002-combined.log
logs/003-combined.log
```

## Inspecting The Capsule

From this directory:

```sh
../../capsule replay cap_23dd3469056c
```

Expected summary:

```text
Commands:
  1. ./gradlew :app:assembleDebug [exit=0]
  2. ./gradlew :app:testDebugUnitTest [exit=0]
  3. ./gradlew :app:lintDebug [exit=0]
Artifacts:
  - app/build/outputs/apk/debug/app-debug.apk (android-apk)
  - app/build/test-results/testDebugUnitTest/TEST-app.capsule.ExampleUnitTest.xml (junit-xml)
  - app/build/reports/lint-results-debug.html (android-lint-report)
  - app/build/reports/lint-results-debug.xml (android-lint-report)
```

## Why This Is Useful

Without Capsule, the sample can only say "the Android build works."

With Capsule, the repo contains a structured execution record showing:

- which commit was tested
- which commands ran
- how long they took
- whether they passed
- where the logs are
- which build/test/lint artifacts were captured
- which local environment produced the run

That is the difference between source-only history and execution history.
