# Frontend implementation and verification

Implemented React/TypeScript/Vite application with Tailwind, Radix accessible dialogs, Lucide icons, locally bundled Monaco workers and uPlot. Seven navigation pages share one native configuration model. Installed binary JSON Schema drives protocol choices, encryption enums, typed fields, nested TLS/users, routing conditions/actions and logical groups. Forms preserve unrecognized properties; configuration check remains authoritative. CRUD, search/sort, dependency-aware deletion, drag/keyboard rule ordering, default outbound, drafts, validation, diff/apply review, history restore, external conflict detection, real SSE telemetry and journald controls are integrated.

The dashboard never synthesizes monitoring data. Disabled/disconnected telemetry displays unavailable values. Local inspection supports validation and drafts, and explicitly disables the Linux-only Apply operation. Mobile <=600px stacks metric cards; light/dark appearance and reduced-motion preferences are supported.

## Sources inspected

- Official route-rule documentation: https://sing-box.sagernet.org/configuration/route/rule/
- Actual installed sing-box 1.14.4 schema generated at `.tools/schema.json` by the integration lead (not bundled as an assumed universal schema).
- Backend API implementation for authenticated endpoints and SSE contracts.

## Commands executed

- `npm install --prefix frontend`: passed with exact pinned dependencies and package lock.
- `npm run lint --prefix frontend`: passed, ESLint recommended JavaScript/TypeScript rules (dynamic schema objects permit explicit any).
- `npm run typecheck --prefix frontend`: passed.
- `npm run test --prefix frontend`: passed, 5 tests covering installed type detection, rule matching/allOf+oneOf merging, no-loss configuration edits, nested dependency detection and unavailable metric formatting.
- `npm run build --prefix frontend`: passed, embedded static assets generated at `backend/internal/web/dist`.
- `npm audit --prefix frontend`: zero vulnerabilities after overriding vulnerable transitive minimatch with pinned 11.0.1.
- Prettier applied to frontend sources and tool configuration.

An initial Vite 6/Vitest 3 dependency set was upgraded to Vite 8.3.6, Vitest 5.1.0, plugin-react 6.2.1 and Tailwind 4.3.4. Installation correctly rejected an incompatible Tailwind peer range before the coherent update. No forced peer-dependency bypass was used. Build warns that the locally bundled Monaco editor produces a large initial JavaScript chunk; no remote CDN is required.

## Files

Created/modified `frontend/package.json`, `package-lock.json`, `tsconfig.json`, `vite.config.ts`, `eslint.config.js`, `index.html`, `src/main.tsx`, `src/App.tsx`, `src/style.css`, `src/vite-env.d.ts`, `src/lib/model.ts`, `src/lib/model.test.ts`, `src/components/SchemaForm.tsx`, `Chart.tsx`, `Logs.tsx`.

Browser QA and actual native API verification are performed by the integration lead and recorded separately. The schema renderer falls back to JSON inputs for arbitrary maps/very deeply nested shapes; the full native JSON editor remains available. Imported unknown fields are never silently removed. A version-changing protocol selection can retain incompatible old fields intentionally; the UI warns and installed validation prevents unsafe application.

## Final accessibility and native-schema audit

- Replaced complex-field nested labels with semantic fieldsets/legends, and assigned unique `useId` / `htmlFor` scalar control labels. Protocol, rule structure, action, and JSON entry controls have stable accessible names.
- Route WHEN controls exclude the complete installed schema's action-field union; the single THEN section contains the action and its options. Nested logical conditions remain visual.
- Raw JSON field inputs set browser validity on malformed values, preventing a form from silently saving its last valid field value.
- Traffic chart X range is explicitly fixed to the selected elapsed one/five/fifteen-minute window, including initial partial windows; absent data is not fabricated.
- Test-only official generated 1.14.4 schema fixture exercises actual protocols, Shadowsocks enum, VLESS users/TLS, route groups, rule-set tags, and selector references. Production continues fetching the installed schema.
- `npm run test --prefix frontend`: 9 passed after this audit. ESLint and production build passed again.

Rule-set tag lists (string or string array) are flattened in reference selectors, preserved when duplicated, displayed explicitly, and all checked for dependencies and uniqueness. The tag-list regression raises the model total to 10 passing tests. Connection detail truncation/unavailability is explicitly distinguished from zero connections. Modal validation errors render inside the active modal so they remain visible above the overlay. Latest lint and production build passed after these changes.

Native rule-set tag lists instantiate distinct rule sets sharing options, rather than alternate names for one set. Installed sing-box validation enforces inline cardinality and local/remote `{tag}` placeholder requirements.

## Monaco lifecycle correction and final browser verification

Real browser testing reproduced two independent disposal defects: the third-party React diff wrapper disposed models before detaching its widget, and Monaco 0.52.2 word-highlighter canceled an unhandled Delayer promise. Replaced the React wrapper with locally owned editor components whose cleanup unsubscribes, detaches models, disposes the editor, then disposes models. The cleanup order has a regression test. Upgraded pinned Monaco to 0.57.0, whose word-highlighter handles cancellation upstream, migrated its JSON API/worker imports, and used its supported textarea input mode (`editContext: false`) for consistent browser input behavior. No page errors are suppressed. Its vulnerable transitive DOMPurify dependency is overridden to fixed 3.4.17; npm audit reports zero vulnerabilities.

- Frontend lint/typecheck/build passed; 11 unit tests passed.
- `go build -o ../artifacts/manager ./cmd/manager` passed from backend.
- `SING_BOX_BIN="$PWD/.tools/sing-box-1.14.4-darwin-arm64/sing-box" node tests/browser/smoke.mjs` passed with actual isolated sing-box and Manager processes: real native API, inbound/outbound/routing/rule-set visual CRUD and installed validation, dependency protection, invalid JSON guard, diff/log controls, three repeated editor/diff mount-unmount cycles, external conflict detection/discard, desktop/light/mobile pages and mobile drawer. No browser exceptions.
- Browser screenshots saved in `docs/screenshots`; mobile capture explicitly scrolls to the page top.
- Light appearance uses explicit white glass materials and dark text colors with a root theme attribute covering portal dialogs. No color-inversion filters are used; chart colors remain intact.

Additional files: `frontend/src/components/CodeEditor.tsx`, `editorLifecycle.ts`, `editorLifecycle.test.ts`; updated `tests/browser/smoke.mjs` with lifecycle cycles and browser error-stack diagnostics. Static editor assets remain local and embedded.

## CI regression at f624cff: global Monaco services

The GitHub run failed inside Monaco 0.57.0 with `InstantiationService has been disposed`, through the global hover factory, ActionBar and deferred SuggestWidget construction. A diff editor overwrote the global hover factory with a closure over its disposable child service. Closing the diff left other editors using that disposed service. The sibling global Markdown renderer had the same lifetime mismatch.

Reproduction in Ubuntu 24.04 ARM64, Node 22.23.4 and real sing-box 1.14.4:

- Native timing was intermittent. Opening a context menu after closing the diff reproduced the stale service reliably.
- Deferring idle work until after diff disposal reproduced the original SuggestWidget stack and subsequent JSON Editor navigation timeout. Failure DOM showed `rootChildren: 0` and no nav buttons: React had unmounted after the error. This was not hidden/obstructed navigation or an asset-loading failure. Local script/style responses and both Monaco worker URLs were captured successfully.
- The patched production build passes the same controlled scheduling scenario and native scheduling. Frontend lint, typecheck, all 11 unit tests, clean npm install and production build pass; npm audit reports zero vulnerabilities.

`frontend/patches/monaco-editor+0.57.0.patch` binds both global factories to the application-lifetime `StandaloneServices` instantiation service. `patch-package --error-on-fail` applies it during npm install/ci. Normal editor/model disposal and all error assertions remain enabled. No timeouts were increased. On dependency upgrades, inspect upstream ownership and remove the patch only after both browser scheduling modes pass unpatched. Related upstream reports: [Monaco #5518](https://github.com/microsoft/monaco-editor/issues/5518), [VS Code #279709](https://github.com/microsoft/vscode/issues/279709).

The browser fixture also reserves both ephemeral ports together, preventing Linux from reusing one port for both Manager and sing-box.

### Reproduce and inspect failures

```sh
npm --prefix frontend ci
npm --prefix frontend run build
bash scripts/ci-browser.sh
```

The browser test runs twice: native idle scheduling and controlled deferred idle scheduling. Both cover post-diff context menus/Find/JSON suggestions, repeated mount/unmount, real native API/schema validation, draft CRUD, conflict protection, and desktop/mobile layouts. The deferred mode is test-only; it does not alter application scheduling.

Failures write `events.json`, `failure.png`, `failure.html`, and `trace.zip` under `.local/browser-artifacts/<timestamp>/`. Events include console arguments/nested errors, uncaught exception stacks, network failures/asset response metadata, worker URLs, child-process output and root/nav visibility/hit-test state. View a trace with `cd tests/browser && npx playwright show-trace /absolute/path/to/trace.zip`. CI and release retain these artifacts for seven days. Traces contain only isolated test-fixture sessions; never run this harness against production.

Set `BROWSER_ARTIFACT_DIR` to override the output directory. Routine screenshots go there too; set `UPDATE_SCREENSHOTS=1` explicitly to refresh tracked documentation screenshots.
