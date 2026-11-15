# USC onboarding image study

Four code-rendered 1080 × 1080 PNGs in `png/`, ordered Brightspace → exam → Leavey → reminders. HTML sources and generator are alongside them. The content is illustrative, not student data. Narration and demo prompts are in narration.md.

Uses the site's Fraunces type and paper/ink/cardinal palette. The generator reads the actual wordmark CSS and helmet/underline SVG paths from the app. Each slide contains only that wordmark and one functional benefit; there are no fabricated product interfaces, headers, footers, or sample data. Fonts load from Google Fonts during export; internet is required. Render with Google Chrome installed:

```
npm install --prefix /tmp/troy-onboarding-render playwright
node design/onboarding/generate.mjs
```

## Message layout

Square is a design choice, not an Apple-specified requirement. The centered composition leaves generous outer clearance for rounded corners and viewer chrome. At 270 px bubble width, 90 px benefit text becomes 22.5 px; at 390 px opened width it becomes 32.5 px. Do not stretch to phone aspect ratio. No baked-in phone chrome or rounded corners.

Apple documents tapping and swiping photo stacks: https://support.apple.com/guide/iphone/share-content-iphb66cfeaad/ios

Grouping, cropping and cover ordering must be verified on the receiving iPhone with the actual transport. Multiple separate sends do not establish a photo stack. Current sendblue.Client.Send is text-only. No automatic connection delivery was added in this design study, and no live iMessage verification is claimed. Before integration, verify multi-attachment support, received ordering, thumbnail crop, opening, and swipe behavior on staging. Do not test against production.
