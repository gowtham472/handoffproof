# Standalone landing on Vercel

Live website: https://handoffproof-landing.vercel.app/

Import gowtham472/handoffproof, branch main. Set Root Directory to `landing`.
Framework preset: Other. Install command: `npm ci`. Build command:
`npm run build:static`. Output directory: `dist`. No environment variables or
backend services are required. Keep the deployment in a personal Hobby project
if its use satisfies Vercel's current non-commercial plan terms.

The static build packages root index.html, /landing scripts/styles/fonts/screens,
and /assets/illustrations. Buttons link to the recorded demo, source and public
documentation. Screenshots remain labelled as captured application previews.
Third-party notices and Storyset attribution are retained.

`npm run build` retains the existing embedded Go build target. Use only
`npm run build:static` for this standalone deployment.
