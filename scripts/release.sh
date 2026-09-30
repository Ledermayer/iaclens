#!/usr/bin/env bash
# Build release archives locally or in Actions. Never publishes.
set -euo pipefail
version="${1:?usage: scripts/release.sh vX.Y.Z[-prerelease]}"
if [[ ! "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$ ]]; then
  echo 'Expected a SemVer tag, e.g. v0.1.0 or v0.2.0-rc.1 (no build metadata).' >&2
  exit 1
fi
if [[ "$version" == *-* ]]; then
  prerelease="${version#*-}"
  if [[ "$prerelease" == .* || "$prerelease" == *. || "$prerelease" == *..* ]]; then
    echo 'Empty prerelease identifier.' >&2; exit 1
  fi
  IFS='.' read -ra identifiers <<< "$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^0[0-9]+$ ]]; then
      echo 'Numeric prerelease identifiers cannot have leading zeroes.' >&2; exit 1
    fi
  done
fi
if [[ "${VALIDATE_ONLY:-false}" == true ]]; then exit 0; fi
commit="$(git rev-parse HEAD)"
build_date="$(git show -s --format=%cI HEAD)"
destination="${RELEASE_DIR:-dist}"
if [[ -e "$destination" ]]; then
  echo "Output directory already exists: $destination. Choose a fresh RELEASE_DIR." >&2
  exit 1
fi
mkdir -p "$destination"
destination="$(cd "$destination" && pwd)"
staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT
go run ./cmd/licensenotices --out "$staging/THIRD_PARTY_NOTICES.txt"
for target_os in linux darwin windows; do
  for target_arch in amd64 arm64; do
    name="iaclens_${version#v}_${target_os}_${target_arch}"
    package="$staging/$name"
    mkdir -p "$package"
    binary=iaclens
    if [[ "$target_os" == windows ]]; then binary=iaclens.exe; fi
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build \
      -trimpath -buildvcs=false \
      -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$build_date" \
      -o "$package/$binary" ./cmd/iaclens
    cp LICENSE README.md SECURITY.md SUPPORT.md CONTRIBUTING.md CHANGELOG.md "$package/"
    cp "$staging/THIRD_PARTY_NOTICES.txt" "$package/"
    cp rules/default.yaml "$package/iaclens.rules.yaml"
    mkdir -p "$package/docs" "$package/schemas"
    cp docs/installation.md docs/report-contract.md "$package/docs/"
    cp schemas/report-v2.schema.json "$package/schemas/"
    # Keep this list aligned with the archive verification below.
    public_docs=(
      LICENSE README.md SECURITY.md SUPPORT.md CONTRIBUTING.md CHANGELOG.md
      iaclens.rules.yaml THIRD_PARTY_NOTICES.txt
      docs/installation.md docs/report-contract.md schemas/report-v2.schema.json
    )
    if [[ "$target_os" == windows ]]; then
      (cd "$package" && zip -q "$destination/$name.zip" "$binary" "${public_docs[@]}")
      unzip -p "$destination/$name.zip" THIRD_PARTY_NOTICES.txt | cmp "$staging/THIRD_PARTY_NOTICES.txt" -
      for doc in "${public_docs[@]}"; do
        unzip -p "$destination/$name.zip" "$doc" | cmp "$package/$doc" -
      done
    else
      tar -czf "$destination/$name.tar.gz" -C "$package" "$binary" "${public_docs[@]}"
      tar -xzOf "$destination/$name.tar.gz" THIRD_PARTY_NOTICES.txt | cmp "$staging/THIRD_PARTY_NOTICES.txt" -
      for doc in "${public_docs[@]}"; do
        tar -xzOf "$destination/$name.tar.gz" "$doc" | cmp "$package/$doc" -
      done
    fi
  done
done
(cd "$destination" && shasum -a 256 ./*.tar.gz ./*.zip > checksums.txt)
