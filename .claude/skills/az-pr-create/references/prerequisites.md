# Prerequisites

- Azure CLI installed: `az --version`
- Azure DevOps extension: `az extension add --name azure-devops`
- Logged in: `az login` (or `az devops login` for PAT-based auth)
- Default org (optional but convenient): `az devops configure --defaults organization=https://dev.azure.com/<org>`

**Do not set a default `project`.** The CLI auto-detects project and repo from the git working directory. Setting a project default breaks commands run from other projects. Pass `--org` and `--project` inline in scripts or CI.