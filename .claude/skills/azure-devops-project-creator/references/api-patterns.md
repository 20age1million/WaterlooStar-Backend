# ADO REST API — Credentials and Utility Functions

---

## Obtaining Credentials

Two values are required: the organisation URL and a Personal Access Token (PAT). Both are
loaded automatically from environment variables by `load_credentials()` in
`references/environment.md`. The user should set these before invoking the skill — they
are never entered in chat.

```
ADO_ORG_URL   https://dev.azure.com/{organisation-name}
ADO_PAT       personal access token (see below)
```

The organisation URL is visible in the browser address bar when logged into ADO, or under
**Organisation Settings → Overview**.

### Creating a PAT

1. Log into Azure DevOps and open **User Settings** (top-right icon)
2. Select **Personal access tokens → New Token**
3. Name it (e.g. `ado-project-creator`) and set the shortest expiry sufficient for the task
4. Under **Scopes**, select **Custom defined** and enable:

| Scope | Permission |
|---|---|
| Project and Team | Read & Write |
| Code | Read & Write |

5. Click **Create** and copy the token — it is not shown again
6. Set it as `ADO_PAT` in the environment before running
7. Revoke the token in ADO once the task is complete

### Security rules

- The PAT is loaded from the environment only — never passed, typed, or logged in chat
- Never hardcode either value in scripts or committed files

---

## Authentication

```python
import base64

def auth_header(pat: str) -> dict:
    token = base64.b64encode(f":{pat}".encode()).decode()
    return {"Authorization": f"Basic {token}", "Content-Type": "application/json"}
```

---

## Project Operations

### poll_operation
ADO project creation is asynchronous. The POST returns `{"url": "...", "status": "notSet"}`.

```python
import time

def poll_operation(op_url: str, pat: str, max_retries=10, delay=3):
    for _ in range(max_retries):
        status = requests.get(op_url, headers=auth_header(pat)).json().get("status")
        if status == "succeeded": return
        if status == "failed": raise RuntimeError("ADO operation failed")
        time.sleep(delay)
    raise TimeoutError("ADO operation timed out")
```

### get_project
```python
def get_project(org_url: str, name: str, pat: str) -> dict | None:
    r = requests.get(
        f"{org_url}/_apis/projects/{name}?api-version=7.1",
        headers=auth_header(pat)
    )
    return None if r.status_code == 404 else r.json()
```

### create_project
```python
PROCESS_TEMPLATES = {
    "agile": "adcc42ab-9882-485e-a3ed-7678f01f66bc",
    "scrum": "6b724908-ef14-45cf-84f8-768b5824c537",
    "cmmi":  "27450541-8e31-4150-9947-dc59f998fc01",
}

def create_project(org_url: str, name: str, description: str, template: str, visibility: str, pat: str) -> str:
    """Returns 'created' or 'existed'."""
    if get_project(org_url, name, pat):
        return "existed"
    r = requests.post(
        f"{org_url}/_apis/projects?api-version=7.1",
        json={
            "name": name, "description": description, "visibility": visibility,
            "capabilities": {
                "versioncontrol": {"sourceControlType": "Git"},
                "processTemplate": {"templateTypeId": PROCESS_TEMPLATES[template.lower()]}
            }
        },
        headers=auth_header(pat)
    )
    r.raise_for_status()
    poll_operation(r.json()["url"], pat)
    return "created"
```

---

## Repository Operations

### get_repo
```python
def get_repo(org_url: str, project: str, repo_name: str, pat: str) -> dict | None:
    r = requests.get(
        f"{org_url}/{project}/_apis/git/repositories/{repo_name}?api-version=7.1",
        headers=auth_header(pat)
    )
    return None if r.status_code == 404 else r.json()
```

### create_repo
```python
def create_repo(org_url: str, project: str, name: str, pat: str) -> tuple[dict, str]:
    """Returns (repo_dict, 'created' | 'existed')."""
    if existing := get_repo(org_url, project, name, pat):
        return existing, "existed"
    r = requests.post(
        f"{org_url}/{project}/_apis/git/repositories?api-version=7.1",
        json={"name": name},
        headers=auth_header(pat)
    )
    r.raise_for_status()
    return r.json(), "created"
```

---

## Commit Operations

### get_latest_sha
Returns the empty-repo sentinel (`"0" * 40`) if the repo has no commits yet.

```python
def get_latest_sha(org_url: str, project: str, repo_id: str, pat: str, branch="main") -> str:
    r = requests.get(
        f"{org_url}/{project}/_apis/git/repositories/{repo_id}/refs?filter=heads/{branch}&api-version=7.1",
        headers=auth_header(pat)
    )
    refs = r.json().get("value", [])
    return refs[0]["objectId"] if refs else "0" * 40
```

### build_change
Constructs a single file-add entry for a push payload.

```python
import pathlib, base64

def build_change(repo_path: str, local_path: str) -> dict:
    content = base64.b64encode(pathlib.Path(local_path).read_bytes()).decode()
    return {
        "changeType": "add",
        "item": {"path": repo_path},
        "newContent": {"content": content, "contentType": "base64Encoded"}
    }
```

### push
Commits one or more changes to a repo in a single push.

```python
def push(org_url: str, project: str, repo_id: str, changes: list, message: str, pat: str):
    r = requests.post(
        f"{org_url}/{project}/_apis/git/repositories/{repo_id}/pushes?api-version=7.1",
        json={
            "refUpdates": [{"name": "refs/heads/main", "oldObjectId": get_latest_sha(org_url, project, repo_id, pat)}],
            "commits": [{"comment": message, "changes": changes}]
        },
        headers=auth_header(pat)
    )
    r.raise_for_status()
```

---

## HTTP Status Reference

| Code | Meaning |
|---|---|
| 401 | Bad or expired PAT — ask user to re-enter |
| 403 | Insufficient PAT scope — confirm Code and Project & Team scopes are enabled |
| 404 | Resource does not exist — safe to use for existence checks |
| 409 | Conflict — resource already exists |
| 429 | Rate limited — back off and retry |

## API Version

Use `api-version=7.1` throughout unless the user's organisation requires otherwise.