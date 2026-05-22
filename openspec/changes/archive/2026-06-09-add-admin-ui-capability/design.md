## Context

Infrastructure presets already declare integration capabilities through `compatibility.provides`, and installation presets use `compatibility.requires` to validate preset composition. Admin UI access is different from an installation prerequisite: it is an optional runtime endpoint that a backend may expose for a concrete deployment.

Today deployment metadata carries `uiPort` beside the SQL port, and connection instructions always render an Administration UI section. That makes UI access look mandatory even though future infrastructure presets may not expose it, and each backend may need to publish a different URL shape.

## Goals / Non-Goals

**Goals:**

- Let infrastructure presets declare Admin UI exposure support with an `admin-ui` capability.
- Represent the resolved Admin UI endpoint as optional deployment metadata.
- Let each supported backend compute the URL in the way that matches its infrastructure, while Exasol Local deployments omit Admin UI metadata until they intentionally expose it.
- Keep SQL connection metadata mandatory and unchanged.
- Avoid runtime commands depending on preset files after deployment.

**Non-Goals:**

- Add an `exasol admin-ui` command or automatically open a browser.
- Change Admin UI authentication or password generation.
- Require every deployment to expose Admin UI.
- Replace existing shell or SQL connection contracts.

## Decisions

1. Presets declare support with `compatibility.provides: ["admin-ui"]`.

   This follows the existing capability vocabulary instead of adding a separate manifest section. The capability means the infrastructure/backend can expose Admin UI for deployments created from that preset. It does not make Admin UI required by installation presets.

   Alternative considered: add an installation `requires: admin-ui`. That would make Admin UI a compatibility requirement and block deployments that are otherwise valid but simply do not expose the UI, which conflicts with conditional exposure.

2. Deployment artifacts carry optional resolved Admin UI details.

   Add an optional object under `connection`, for example:

   ```json
   {
     "connection": {
       "host": "127.0.0.1",
       "dbPort": 8563,
       "adminUi": {
         "url": "https://127.0.0.1:8443",
         "username": "admin",
         "insecureSkipCertValidation": true
       }
     }
   }
   ```

   Runtime commands consume this object and do not re-read preset manifests. This keeps `deployment.json` as the source of truth for the concrete deployment.

   Alternative considered: keep `uiPort` as the only UI contract and synthesize URLs in connection instructions. That works only when the UI uses the same host scheme as SQL access and does not model backend-specific URLs cleanly.

3. Preserve legacy `uiPort` reading as a fallback during normalization.

   Existing deployment artifacts and cloud outputs already include `connection.uiPort` and `nodes[*].database.uiPort`. Normalization should derive `connection.adminUi` from those fields when no explicit object exists, so older deployments remain readable while new deployments can use the clearer contract.

4. Backend-specific resolution stays at the backend/preset boundary.

   Cloud tofu presets should emit provider-specific `connection.adminUi.url` in their `outputs.tf`. The local backend should omit Admin UI metadata because the local preset does not advertise the capability. Generic deployment commands should only render what the deployment artifact contains.

## Risks / Trade-offs

- Existing deployments may only have `uiPort` metadata -> keep fallback normalization and tests for legacy artifacts.
- Some custom presets may advertise `admin-ui` but fail to emit `connection.adminUi` -> connection instructions should simply omit the UI section if the resolved endpoint is absent.
- Admin UI certificate behavior may differ by backend -> keep certificate validation metadata on the Admin UI object rather than assuming it matches SQL connection validation.
