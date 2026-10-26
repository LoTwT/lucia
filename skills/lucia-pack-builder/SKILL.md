---
name: lucia-pack-builder
description: Build, preview, validate, or install a Lucia desktop-pet pack from prepared transparent animated WebP clips and a pack.source.json. Use when the user wants to turn their own artwork into a Lucia pack, add clickable hotzones or bubbles, check a source directory, or produce an installable pack zip. Do not use for generic WebP conversion or for editing an already installed pack.json.
---

# Lucia Pack Builder

Build a Lucia pet pack with the repository's deterministic builder. The contract lives in `docs/specs/pet-pack-source.md`; read it before editing source metadata or explaining field behavior.

## Workflow

1. Locate the Lucia repository and the user's source directory. Confirm `go run ./cmd/petpack build -help` works.
2. Inspect the source:
   - `pack.source.json` and `clips/` are expected.
   - Require clip ID `idle_loop`; its default filename is `idle_loop.webp`, but `clips` may map it to another WebP file.
   - Ask for missing `id`, `name`, and license information. Never infer a license.
   - Reject non-WebP animation inputs; this skill does not convert PNG, GIF, APNG, or MP4.
3. Preview the artwork:
   ```sh
   go run ./cmd/petpack preview <source-dir> -o /tmp/lucia-pack-preview.png
   ```
   The preview is the full canvas at native resolution, not the cropped runtime frame. Use it for appearance and hotzone measurements; read the printed `canvas` and `crop` values and express hotzone coordinates in canvas pixels. Do not treat scaled/cropped preview pixels as manifest coordinates.
4. If the user wants clickable regions:
   - Express `hotzones[].rect` in canvas pixels, not crop-normalized coordinates.
   - A hotzone may contain `steps`, `bubble`, `sound`, `motion`, and `reducedPoster`.
   - Use only `rect`; do not invent ellipse or polygon support.
   - Pin `crop` when the pack has hotzones and the source is likely to change; otherwise warn about crop drift. If an unpinned automatic crop excludes a hotzone, the build fails; pin a crop that contains the region or move the rect.
   - Write coordinates only after the user confirms them.
5. Build:
   ```sh
   go run ./cmd/petpack build <source-dir> -o <pack.zip>
   ```
   Report the clip, frame, action, hotzone, and interaction counts, plus every warning. A successful build with `0 hotzones` means the pet is not clickable.
6. Install only after an explicit request:
   - Ask the user to close the running Lucia app.
   - Confirm the target data directory; do not silently pick one.
   - Install with activation disabled first:
     ```sh
     go run ./cmd/petpack -zip <pack.zip> -data <data-dir> -activate=false
     ```
   - Activate only after separate confirmation.

## Guardrails

- Do not write `pack.json`, clips, hashes, or zip bytes by hand. You may write a user-confirmed crop in `pack.source.json` using canvas pixels; never hand-edit the runtime manifest's normalized crop.
- Do not use `pack.source.json` as a runtime manifest or copy runtime-derived fields into it.
- Do not restore the removed `faceClickable` field or invent default face/head hotzones.
- Do not embed naiwa or other user artwork in the repository or CI.
- Keep build and install separate; `build` must not modify the user's data directory.
- If the builder rejects a source, report the exact validation error and the file/field it names. Do not retry with guessed coordinates or fabricated metadata.

## Validation

The builder already performs strict source decoding, alpha/canvas checks, manifest validation, deterministic zip generation, and a build-to-import roundtrip. For an installed directory, run:

```sh
go run ./cmd/petpack -check <packs-dir>/<id>
```

Finish with the artifact path, summary counts, warnings, and whether installation was requested.
