# Troubleshooting

Common setup issues encountered when running Gophish-NG in Docker, along with their fixes.

## Container won't start: `exec: "./docker/run.sh": no such file or directory`

**Symptom:** `docker compose up` fails with an OCI runtime error like:

```
failed to create shim task: ... exec: "./docker/run.sh": stat ./docker/run.sh: no such file or directory
```

**Cause:** The `/opt/gophish` directory inside the container holds both the running binary and the startup script. A volume mounted over the whole path, e.g. `-v ./data:/opt/gophish`, **shadows the binary and `docker/run.sh`**, so the container has nothing to execute.

**Fix:** Mount only a subdirectory and point the database there:

```yaml
volumes:
  - ./data:/opt/gophish/data
environment:
  DB_PATH: /opt/gophish/data/gophish.db
```

## Admin panel not reachable / TLS handshake dies when binding to loopback

**Symptom:** With `ADMIN_SERVER_LISTEN_URL=127.0.0.1:3333`, the admin UI is unreachable from the host (`Connection reset`, `curl: (35) SSL connect error`, `unexpected eof` during the handshake) even though the log says the server started. **Applies to Docker Desktop (macOS/Windows);** on native Linux Docker the loopback binding usually works as expected.

**Cause:** On Docker Desktop the port-forward proxy cannot forward a host loopback port onto a container port that is bound to the container's own `127.0.0.1`. Mid-handshake connection drops are the tell.

**Fix:** Let the container listen on all interfaces and restrict exposure on the host side with the port mapping:

```yaml
environment:
  ADMIN_SERVER_LISTEN_URL: 0.0.0.0:3333
ports:
  - "127.0.0.1:3333:3333"
```

## The admin password is only printed once — and it's random

**Symptom:** On first boot the log shows `Please login with the username admin and the password <random>`. After a restart (or after the log rotates) the password is no longer recoverable.

**Cause:** The admin password is generated at first database migration and persisted in the DB; it is never re-printed on later boots.

**Fix:** Keep the first-boot log line (`docker compose logs | grep "Please login"`), or reset the database to force a fresh password:

```bash
docker compose down
rm -rf data          # your mounted data dir
docker compose up -d
docker compose logs | grep "Please login"
```

## Campaign "Launch" button spins forever and nothing happens

**Symptom:** Clicking **Launch Campaign** shows a spinner; no campaign appears in the list, no email is sent, and the server never receives a `POST /api/campaigns/`.

**Cause:** All four fields on the campaign form must be populated — **Sending Profile**, **Email Template**, **Landing Page**, and **Target Group**. Client-side validation silently blocks submission when one is missing (a target group is required even for a local mail sink test, and one is *not* created by default).

**Fix:** Create a target group (with a placeholder address if only testing delivery), then fill every dropdown on the form and relaunch.

## "You need a JavaScript enabled browser to view this site"

**Symptom:** Visiting the tracked campaign URL shows only this message in some preview windows.

**Cause:** Gophish landing pages render a `<noscript>` fallback with this text for pages that rely on JavaScript for click tracking / redirect. Mailpit's preview pane (and any JS-less browser context) cannot execute the script, so the fallback is all you see.

**Fix:** Open the click URL in a regular browser tab with JavaScript enabled. This is expected behavior, not an error.

## Clicking the link in a test email opens Mailpit instead of the landing page

**Symptom:** The tracked link hits `http://localhost:8025` (the Mailpit UI) instead of the phish server, so no click is recorded.

**Cause:** The email template linked straight to Mailpit (`http://localhost:8025`) instead of using Gophish's placeholder `{{.URL}}`, which is what injects the per-recipient tracking URL (`<phis-server>?rid=<id>`).

**Fix:** In the email template, replace the hardcoded link with `{{.URL}}`.

## Submitted data recorded but no password values

**Symptom:** A landing page captures a submission (event logged) but the password fields are blank in the results.

**Cause:** **Capture Submitted Data** logs the request and non-password fields; password values are only stored when the landing page also has **Capture Passwords** enabled.

**Fix:** Enable **Capture Passwords** on the landing page to include password field values in the captured data.
