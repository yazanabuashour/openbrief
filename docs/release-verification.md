# Release Verification

Tagged OpenBrief releases publish:

- `openbrief_<version>_<os>_<arch>.tar.gz`
- `openbrief_<version>_skill.tar.gz`
- `openbrief_<version>_source.tar.gz`
- `openbrief_<version>_checksums.txt`
- `openbrief_<version>_sbom.spdx.json`
- `install.sh`

The platform archives contain the production `openbrief` binary. The skill archive contains the shipped `SKILL.md`. The source archive is the canonical Go module and local runtime source artifact.

The installer verifies the matching platform archive, installs the same-tag runner, prints `openbrief --version`, and tells users to register the same-tag skill source or archive with their agent. Checksums and GitHub attestations verify that release assets were produced by this repository's workflow.

The release workflow publishes through a draft-first path and verifies the draft asset set before publication, so future GitHub immutable releases can lock tags and assets only after every release asset and attestation is ready.

## Verify a Release

Download the assets from the GitHub Release page for the tag you want to verify, then run:

```bash
shasum -a 256 -c openbrief_<version>_checksums.txt
gh attestation verify openbrief_<version>_<os>_<arch>.tar.gz --repo yazanabuashour/openbrief
gh attestation verify openbrief_<version>_skill.tar.gz --repo yazanabuashour/openbrief
gh attestation verify openbrief_<version>_source.tar.gz --repo yazanabuashour/openbrief
gh attestation verify install.sh --repo yazanabuashour/openbrief
```

If these commands succeed, the assets match the published checksums and have valid GitHub attestations for this repository.

For the latest release, verify GitHub's latest pointer resolves to the expected tag:

```bash
gh release view --repo yazanabuashour/openbrief --json tagName --jq .tagName
```

When repository-level release immutability is enabled, published release tags and assets cannot be replaced after publication. If an artifact is wrong, ship a new patch release instead of mutating the existing release.

## Smoke-Test an Install

Install into a temporary directory, then verify the runner version and commands:

```bash
install_dir="$(mktemp -d)"
OPENBRIEF_INSTALL_DIR="$install_dir" \
  OPENBRIEF_VERSION=v0.1.0 \
  sh -c "$(curl -fsSL https://github.com/yazanabuashour/openbrief/releases/download/v0.1.0/install.sh)"

export PATH="$install_dir:$PATH"
command -v openbrief
openbrief --version
openbrief --help
```

The valid runner commands are `config` and `brief`.

## SBOM

The SPDX JSON SBOM asset is intended for audit tooling and manual inspection:

```bash
jq '.packages | length' openbrief_<version>_sbom.spdx.json
```

The SBOM is generated from the tagged source contents during the release workflow and attached to the same GitHub Release as the binary, skill, and source archives.
