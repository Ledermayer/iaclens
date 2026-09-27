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
    cp LICENSE README.md "$package/"
    if [[ "$target_os" == windows ]]; then
      (cd "$package" && zip -q "$destination/$name.zip" "$binary" LICENSE README.md)
    else
      tar -czf "$destination/$name.tar.gz" -C "$package" "$binary" LICENSE README.md
    fi
  done
done
(cd "$destination" && shasum -a 256 ./*.tar.gz ./*.zip > checksums.txt)
