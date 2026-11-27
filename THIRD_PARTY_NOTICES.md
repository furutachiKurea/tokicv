# Third-Party Notices

This repository redistributes the following third-party assets. The full
license texts are stored in `LICENSES/`.

## Nunito

- Files: `internal/render/assets/fonts/nunito-latin-400-normal.woff2`,
  `nunito-latin-500-normal.woff2`, `nunito-latin-600-normal.woff2`,
  `nunito-latin-700-normal.woff2`
- Upstream: The Nunito Project Authors,
  https://github.com/googlefonts/nunito
- Distribution: static WOFF2 files bundled with this repository, font version 3.601
- License: SIL Open Font License 1.1
- License text: `LICENSES/Nunito-OFL-1.1.txt`
- SHA-256:
  - `nunito-latin-400-normal.woff2`: `a8e095cdd646a5c74f870a1896dcbfa3f6f97e42b3525ffb0b918fb04c79243c`
  - `nunito-latin-500-normal.woff2`: `7a260295373448f856c9e357e0bce95c399a271e3090f5fd49772e4a99a78680`
  - `nunito-latin-600-normal.woff2`: `2c8f6b581f62330fa50190822ec8fdee21c8829fbad61b87543970dc677c60fb`
  - `nunito-latin-700-normal.woff2`: `d9b593e4cff750b77f0679694605c328a9229d1869f8cd18bfaddefff7a3cd80`

## PDF.js

- Files: `internal/render/assets/pdfjs/pdf.min.mjs`,
  `internal/render/assets/pdfjs/pdf.worker.min.mjs`
- Package: `pdfjs-dist` 6.5.438 (legacy build)
- Upstream: https://github.com/mozilla/pdf.js
- License: Apache License 2.0
- License text: `LICENSES/PDF.js-Apache-2.0.txt`
- SHA-256:
  - `pdf.min.mjs`: `522ad2c83bc8a0010e7f0cc60d0892b77c8d2bf2c9702dc12288311d94270f42`
  - `pdf.worker.min.mjs`: `b4c8353ded0d88d2e2009574a29eb4e0c3ff7e942ecdf51ef5f71566a9667f31`

The embedded PDF.js files retain their upstream Apache License 2.0 notice
headers.

## Matt Pocock agent skills

- Files: `.agents/skills/code-review/**`,
  `.agents/skills/codebase-design/**`,
  `.agents/skills/diagnosing-bugs/**`,
  `.agents/skills/domain-modeling/**`,
  `.agents/skills/improve-codebase-architecture/**`,
  `.agents/skills/retro/**`, and `.agents/skills/tdd/**`
- Upstream: https://github.com/mattpocock/skills
- Revision: `6f7f7831a625f040a8e06a0e7954826b87ce775a`
- Distribution: skill instructions and support files copied verbatim into
  this repository; the TokiCV factory mapping is local to
  `.agents/skills/README.md`
- License: MIT
- License text: `LICENSES/Matt-Pocock-skills-MIT.txt`
- SHA-256:
  - `code-review/SKILL.md`: `9c921a8d88138a41401d8897ca1bb852088ab1b7b605cdd04edc13e3e8aab6dd`
  - `code-review/agents/openai.yaml`: `8229ca854e11dc8e6aef2131ee03f31fb1561cf905fab9ccc325180cf3331352`
  - `codebase-design/DEEPENING.md`: `f3dd099ce99289bd213914d8ee3e2429b78309c3957ca4583f7659551b1d53c1`
  - `codebase-design/DESIGN-IT-TWICE.md`: `c222ab5a82e0405672eb9062d62496139a08828e45d07fc5f18e4952c10fac1a`
  - `codebase-design/SKILL.md`: `2c20617f87ec8af6a434859f381b2f061a69b530444e74eb39e78bb016a6d1e2`
  - `codebase-design/agents/openai.yaml`: `edebc9e4fcfe102114012575eaa9600b9b5fd08c311664f389c36e7bc717740f`
  - `diagnosing-bugs/SKILL.md`: `849198df88cdb65fd0d97236604c1fca7ff2f22d2f705f9724a800d0266b118d`
  - `diagnosing-bugs/agents/openai.yaml`: `3e430dbe4334a87597488c060cb3dc3786bb00c9182877d6f5ec41f62490e90b`
  - `diagnosing-bugs/scripts/hitl-loop.template.sh`: `35103539fc36873eea36074769ad454f9379d6fc8b2dc0e26ce987fd3bfe5503`
  - `domain-modeling/ADR-FORMAT.md`: `944c92aa790e8fbdc9199640b170979abb8a34ba8d0fe18c2a01a63bce140ca0`
  - `domain-modeling/GLOSSARY-FORMAT.md`: `479bb4d9cae0ef5cca8fa23ffb5ed5b1b93b41490878315551ceb0c727146cb6`
  - `domain-modeling/SKILL.md`: `7b925d7b1e341a2eeae33ad68a8a8c0ab889a38ddd22a7598e4c240e5a7556a3`
  - `domain-modeling/agents/openai.yaml`: `f6bf2aa996c6e6f53fdd0708e18a0d16a56aed8322cca59fedbe3c0d2c75f06b`
  - `improve-codebase-architecture/HTML-REPORT.md`: `581e8bb5a521e46bbda8ca7e19b15948bed882187108092ebb90c62513b77528`
  - `improve-codebase-architecture/SKILL.md`: `35b03a3db61e0f5aa333ea59b54963e45a3d2ace6649b2ef4f9576603e2582a6`
  - `improve-codebase-architecture/agents/openai.yaml`: `c8cb20f68ebf0edb4e497bc11ae5fcaa196004e661cd189015b04f4109ced7f1`
  - `retro/SKILL.md`: `33fbc28568e3146f20fd908b12998bc0ab30aa4f4c87288199a40793efa94696`
  - `retro/agents/openai.yaml`: `cb6fb9d7e6b4be5975f41f6def1dd00a1255bad5d9b02696b124c924a8b8745c`
  - `tdd/SKILL.md`: `5505d25bbaa8fc14a79e4d20163936958992a2995b7f1dd1d6197edc9ed722e0`
  - `tdd/agents/openai.yaml`: `ea6f01cf1b8c06a4b0f5b649d74b1b8ce8685e72af1b38d70d877693e092af0b`
  - `tdd/mocking.md`: `3ceb807fdf4a47d6a93d4d9a891e5ba6d362a6247bd08adc451feebfc17361ef`
  - `tdd/tests.md`: `859f9e592c188fda4fc7277dd180e4ce9c7a2e13f6efe1f6f29eccc9d28c106a`
