# petpack test fixture

`tiny.webp` is a 3-frame 16×16 animated WebP generated with `img2webp` from
solid-color PNGs created for this repository. It is original test data, not
third-party artwork. The frames intentionally use partial rectangles, one
dispose-to-background frame and one alpha-blended frame so the decoder tests
cover the ANMF composition rules.

`tiny-lossy.webp` is the same animation encoded with lossy VP8 + ALPH, to
cover the animated WebP form that is not used by the naiwa pack but is valid
in the manifest format.
