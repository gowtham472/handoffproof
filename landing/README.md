# HandoffProof landing page

The Go server embeds the production files in `cmd/handoffproof/web/landing` and
serves the page at `/landing`. It uses the same operator authentication as the
console. The existing console remains at `/`, preserving OAuth and deep links.

Build after changing this source:

```sh
cd landing
npm ci
npm run typecheck
npm run build
```

Commit the generated bundle together with source changes, so Go builds do not
require Node or access to an asset CDN. The landing page is informational;
its preview shows actual product screenshots captured against recorded demo data.
Screenshots are snapshots, not live measurements. See `screens/README.md`.

The components in `src/reactbits` are copied unchanged from the user-provided
`D:/H-projects/code2play/src/components/reactbits` reference. They are the reference's
React Bits implementations: LaserFlow, ShinyText, Magnet, StarBorder, CountUp,
SpotlightCard, GlareHover, ClickSpark, and DecryptedText. Original project:
https://reactbits.dev (https://github.com/DavidHDev/react-bits).
Existing source comments are retained. HandoffProof copy and page composition
are separate from these components.

The upstream MIT + Commons Clause notice is retained in
`src/reactbits/LICENSE.md`, the compiled JavaScript header, and the served
`THIRD_PARTY_NOTICES.txt`. The components are used as part of this application.

Plus Jakarta Sans is self-hosted from `@fontsource-variable/plus-jakarta-sans`.
Phosphor React components supply the landing icons; `console-icons.mjs` generates
the same package's SVG paths for the buildless console. The generated sprite,
font, and their license notices ship with the embedded landing assets.
