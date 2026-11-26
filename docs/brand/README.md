# clump — logo guidelines

## The logo

A lowercase **c** built from diff lines: seven rounded bars on one circular left edge, ragged on the
right like hunk lines. Coloured like a diff hunk: three removed lines in red on top, one unchanged
line in grey, three added lines in green below. The bars are the same shape as the lines clump
shows you.

Wordmark: `clump` in lowercase, drawn as one even-weight line with round ends, the same ends as the bars.

## Files

| File | Use |
|---|---|
| `clump-symbol.svg` | Master symbol, full colour. 32 px and up. |
| `clump-symbol-small.svg` | Five thicker bars. Use below 32 px (favicon, tabs, tiny avatars). |
| `clump-horizontal.svg` / `clump-stacked.svg` | Lockups on light backgrounds (ink wordmark). |
| `clump-horizontal-dark.svg` / `clump-stacked-dark.svg` | Lockups on dark backgrounds (white wordmark). |
| `clump-wordmark.svg` | Wordmark only. |
| `variants/` | One-colour black, white and green versions of the above. |
| `web/` | `favicon.svg`, `favicon.ico`, PNG icons, `site.webmanifest`, `head-snippet.html`. |

The wordmark is stroke-based (one `<g>` with `stroke-width`), not outlined. Keep the SVG as is; don't
retype it in a font.

## Colour

| Name | HEX | RGB | Use |
|---|---|---|---|
| Removed (red) | `#E5484D` | 229 · 72 · 77 | Top three bars |
| Context (grey) | `#9AA0B4` | 154 · 160 · 180 | The middle, leftmost bar |
| Added (green) | `#2BB673` | 43 · 182 · 115 | Bottom three bars |
| Ink | `#16141F` | 22 · 20 · 31 | Wordmark on light; app-icon tile; theme colour |
| White | `#FFFFFF` | | Wordmark on dark |

CMYK and Pantone values are not set here: match them from the HEX with your printer.

Approved pairs: full colour on white, on `#F6F6F2`, and on Ink. On anything else (photos, other
colours) use the one-colour black or white version.

Red and green are the same lightness to some colour-blind viewers, so the logo never relies on hue
alone: red is always on top and green always below, and the greyscale and one-colour versions still read.
Keep text out of the logo colours; green on white is about 2.5:1.

## Clear space and minimum size

- Clear space: one bar height of the symbol (22 units of 256), scaled with the logo, on every side.
- Symbol: full version from 32 px, small cut from 16 px.
- Horizontal lockup: 96 px wide.

## Don't

- Don't swap the order (red stays on top, green below) or recolour bars outside the palette.
- Don't use the full symbol below 32 px; use the small cut.
- Don't stretch, rotate, outline, shadow or add gradients.
- Don't put the ink wordmark on dark or the white wordmark on light.
- Don't retype the wordmark in a font.

## Notes

- Not trademark-checked. Run a search before putting it on anything commercial.
- Sources: the symbol is plain `<rect>` elements; edit those rather than re-tracing.
