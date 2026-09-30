# Install IaCLens

Download binaries from [GitHub Releases](https://github.com/Ledermayer/iaclens/releases).
The commands below use the published `v0.1.0-rc.3` prerelease. Use `v0.1.0` only
after that release exists; do not assume a stable release is already available.
Private-repository downloads require GitHub authentication. Once public, downloads
do not require an account.

## Requirements And Packages

Git must be installed and available on `PATH` for analysis, even with a prebuilt
binary. Go is not needed to run a downloaded binary. Source builds require
Go 1.27.1 or later. Terraform, cloud credentials and model credentials are not
needed for offline analysis. Live Jev mode is optional and requires an account
and key with the selected provider.

| Platform | Architecture | Archive suffix |
| --- | --- | --- |
| macOS, Apple Silicon | arm64 | `darwin_arm64.tar.gz` |
| macOS, Intel | amd64 | `darwin_amd64.tar.gz` |
| Linux, x86-64 | amd64 | `linux_amd64.tar.gz` |
| Linux, ARM64 | arm64 | `linux_arm64.tar.gz` |
| Windows, x86-64 | amd64 | `windows_amd64.zip` |
| Windows, ARM64 | arm64 | `windows_arm64.zip` |

Archive names are `iaclens_<version-without-v>_<suffix>`. Download that archive
and `checksums.txt` from the same release. SHA-256 checksums detect corruption and
inconsistent downloads; they are not signatures or independent proof of origin.
Binaries are currently unsigned and macOS binaries are not notarized. Follow your
organization's software-approval process if Gatekeeper or SmartScreen intervenes;
do not disable system-wide protections to install IaCLens.

## macOS And Linux

Run this in a new download directory with Bash or Zsh. Set the architecture and
operating system using the table above (`uname -m` and `uname -s` help identify
them). This example installs the macOS Apple Silicon package:

```sh
VERSION=0.1.0-rc.3
TARGET_OS=darwin
TARGET_ARCH=arm64
ARCHIVE="iaclens_${VERSION}_${TARGET_OS}_${TARGET_ARCH}.tar.gz"
BASE_URL="https://github.com/Ledermayer/iaclens/releases/download/v${VERSION}"

# Public repository download. For private access, use the command below instead.
curl --fail --location --output "$ARCHIVE" "$BASE_URL/$ARCHIVE"
curl --fail --location --output checksums.txt "$BASE_URL/checksums.txt"
```

Before public availability, replace the two `curl` commands with authenticated
GitHub CLI download (GitHub CLI is a download convenience, not a runtime dependency):

```sh
gh release download "v${VERSION}" --repo Ledermayer/iaclens \
  --pattern "$ARCHIVE" --pattern checksums.txt
```

Verify the selected archive before extraction:

```sh
EXPECTED=$(awk -v name="./$ARCHIVE" '$2 == name { print $1 }' checksums.txt)
if [ "${#EXPECTED}" -ne 64 ]; then
  printf 'Missing or ambiguous archive checksum\n' >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  printf '%s  %s\n' "$EXPECTED" "$ARCHIVE" | sha256sum --check -
else
  printf '%s  %s\n' "$EXPECTED" "$ARCHIVE" | shasum -a 256 --check -
fi
# Continue only if verification reports OK.
tar -xzf "$ARCHIVE"
./iaclens --version
mkdir -p "$HOME/.local/bin"
install -m 0755 iaclens "$HOME/.local/bin/iaclens"
export PATH="$HOME/.local/bin:$PATH"
iaclens --version
```

Persist that `PATH` entry in your shell profile if needed. Retain the extracted
documentation, license and third-party notices with the download. If checksum
verification fails, discard the download and investigate; do not run it.

## Windows PowerShell

Run from a new download directory. Choose `arm64` instead of `amd64` for an ARM64
Windows machine. Git for Windows must be installed separately and on `PATH`.

```powershell
$ErrorActionPreference = 'Stop'
$Version = '0.1.0-rc.3'
$Architecture = 'amd64'
$Archive = "iaclens_${Version}_windows_${Architecture}.zip"
$BaseUrl = "https://github.com/Ledermayer/iaclens/releases/download/v$Version"

# Public repository download. For private access, use the gh command below instead.
Invoke-WebRequest "$BaseUrl/$Archive" -OutFile $Archive
Invoke-WebRequest "$BaseUrl/checksums.txt" -OutFile checksums.txt
```

Private download alternative:

```powershell
gh release download "v$Version" --repo Ledermayer/iaclens --pattern $Archive --pattern checksums.txt
if ($LASTEXITCODE -ne 0) { throw 'Download failed' }
```

Verify, extract, and install for the current user:

```powershell
$Entries = @(Get-Content checksums.txt | Where-Object {
    ($_ -split '\s+', 2)[1] -ceq "./$Archive"
})
if ($Entries.Count -ne 1) { throw 'Missing or ambiguous archive checksum' }
$Expected = ($Entries[0] -split '\s+', 2)[0]
$Actual = (Get-FileHash $Archive -Algorithm SHA256).Hash
if ($Expected -notmatch '^[0-9a-fA-F]{64}$' -or $Actual -ine $Expected) {
    throw 'Checksum mismatch; do not run the download'
}
Expand-Archive $Archive -DestinationPath extracted
& .\extracted\iaclens.exe --version
if ($LASTEXITCODE -ne 0) { throw 'Binary smoke test failed' }
$InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\IaCLens'
New-Item -ItemType Directory -Force $InstallDir | Out-Null
Copy-Item .\extracted\* $InstallDir -Recurse -Force
$env:Path = "$InstallDir;$env:Path"
iaclens --version
```

For future terminals, add `$InstallDir` to the user's Path through Windows
Environment Variables. No administrator installation is required.

## First Offline Scan

This uses public Terraform source, with no API key or cloud account. It does not
execute Terraform or deploy resources. Run in Bash, Zsh, or PowerShell:

```sh
git clone --depth 1 https://github.com/Azure/terraform-azurerm-avm-res-resources-resourcegroup.git resourcegroup
iaclens --print-default-config > trusted-rules.yaml
iaclens --source resourcegroup --config trusted-rules.yaml --kind module --mode offline --format json --out resourcegroup.json
```

PowerShell users should check `$LASTEXITCODE` after native commands. `--source`
always resolves the containing Git root: it is not a subtree-only scan.
`--kind module` applies to the root Terraform directory only. Nested components
can remain unknown offline; use reviewed per-path overrides where appropriate.
The explicit config prevents upstream `.iaclens.yaml` from replacing your policy.

Read the output file, not stdout, for the JSON report. Exit 0 means analysis
completed, not that every check passed. See the [report contract](report-contract.md)
before using results as a CI gate or publishing them.

## Upgrade, Remove, Or Build

For an upgrade, download a specific new release into a fresh directory, verify
its checksum, run `--version`, and replace the old binary and accompanying docs.
Keep the previous verified download for rollback. Installed binaries do not
self-update; dependency PR merges do not update an existing download.

To uninstall, remove the binary you installed under `$HOME/.local/bin`, or remove
the dedicated `%LOCALAPPDATA%\Programs\IaCLens` directory on Windows. Remove only
the Path entry added for IaCLens, and retain or delete your own reports/configs
according to your data-retention policy. There is no service to stop.

For a source build, clone the IaCLens repository, check out a reviewed tag, then:

```sh
go build -o work/iaclens ./cmd/iaclens
go test ./...
```

On Windows use `go build -o work/iaclens.exe ./cmd/iaclens`. Plain source builds
report `dev`; release packaging supplies version/commit metadata. No Homebrew,
winget, apt, or other package-manager distribution is currently maintained.

## Troubleshooting

- `git` or `iaclens` not found: check installation and `PATH`, then open a new terminal.
- Wrong CPU/OS error: choose the platform-specific archive, not just a matching extension.
- `--source must be inside a Git worktree`: use a cloned or initialized Git repository.
- `--kind requires Terraform files`: use `auto` and per-path overrides for a rootless repository.
- Unknown classifications or unchecked checks offline: this is uncertainty, not a successful quality assessment.
- Missing model key: stay offline or configure the chosen provider as described in the [README](../README.md).
- Live timeout/size failure: scope collection with reviewed file/path filters; do not publish an older output as a fresh result.

For safe bug reports and questions, see [Support](../SUPPORT.md).
