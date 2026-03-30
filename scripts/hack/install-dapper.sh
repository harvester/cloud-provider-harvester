#!/bin/bash
set -e

# Download dapper per version
# Hard-code the sha512 checksum to validate the downloaded file (zero-trust)
# When updating to newer version, replace them

# 1. Configuration
VERSION="0.6.0"

# Prioritize ENV variable 'OS', fallback to uname
DETECTED_OS=$(uname -s)
OS="${OS:-${DETECTED_OS}}"

# Prioritize ENV variable 'ARCH', fallback to uname
DETECTED_ARCH=$(uname -m)
TARGET_ARCH="${ARCH:-${DETECTED_ARCH}}"

# 2. Map OS and Architecture
# We normalize the string to handle case-sensitivity and common aliases
case "${OS}_${TARGET_ARCH}" in
  Linux_x86_64|Linux_amd64)
    REAL_OS="Linux"
    REAL_ARCH="x86_64"
    EXPECTED_SHA512="d2ec27ecf9362e2fafd27d76d85a5c5b92b53aefe07cffa76bf9887db6bee07b1023cca8fc32a2c9bdd2ecfadaee71397066b41bd37c9ebbbbce09913f0884d4"
    ;;
  Linux_aarch64|Linux_arm64)
    REAL_OS="Linux"
    REAL_ARCH="aarch64"
    EXPECTED_SHA512="781951b31e5ff018a04e755c6da7163b31a81edda61f1bed4def8d0e24229865c58a3d26aa0cc4184058d91ebcae300ead2cad16d3c46ccb1098419e3e41a016"
    ;;
  Darwin_arm64|Darwin_aarch64)
    REAL_OS="Darwin"
    REAL_ARCH="arm64"
    EXPECTED_SHA512="8a356c89ad32af1698ae8615a6e303773a8ac58b114368454d59965ec2aa8282e780d1e228d37c301ce6f87596f68bfe7f204eb5f4c019c386a58dd94153ddcf"
    ;;
  Darwin_x86_64|Darwin_amd64)
    REAL_OS="Darwin"
    REAL_ARCH="x86_64"
    EXPECTED_SHA512="dbab05de04dda26793f4ae7875d0fba96ee54b0228e192fd40c0b2116ed345b5444047fc2e0c90cb481f28cbe0e0452bcecb268c8d074cd8615eb2f5463c30b6"
    ;;
  Windows_x86_64|Windows_amd64)
    REAL_OS="Windows"
    REAL_ARCH="x86_64"
    EXPECTED_SHA512="807aee2f68b6da35cb0885558f5cbc9a6c8747a56c7a200f0e1fcac9e2fd0da570cbb39e48b3192bd1a71805f2ab38fd19d77faebba97a89e5d9a8b430ee429e"
    SUFFIX=".exe"
    ;;
  *)
    echo "ERROR: Unsupported OS/Arch: ${OS}_${TARGET_ARCH}" >&2
    exit 1
    ;;
esac

FILENAME="dapper-${REAL_OS}-${REAL_ARCH}${SUFFIX}"

DOWNLOAD_URL="https://releases.rancher.com/dapper/v${VERSION}/${FILENAME}"

# 3. --- Checksum Tool Detection ---
if command -v sha512sum >/dev/null 2>&1; then
  # Linux/GNU path
  CHECK_CMD="sha512sum -c"
elif command -v shasum >/dev/null 2>&1; then
  # macOS/BSD path
  CHECK_CMD="shasum -a 512 -c"
else
  echo "ERROR: Neither sha512sum nor shasum found. Cannot verify download." >&2
  exit 1
fi

# 4. Use >&2 to ensure these print even if stdout is being captured
echo "--- Downloading Dapper v${VERSION} (${REAL_ARCH}) ---" >&2

curl -sSL -f -o .dapper.tmp "${DOWNLOAD_URL}"

# 5. Verify
echo "--- Verifying Checksum $EXPECTED_SHA512 ---" >&2
# We pipe the hash to the command and redirect ALL output to /dev/null.
# The 'if' statement then checks the exit code.
if echo "${EXPECTED_SHA512}  .dapper.tmp" | ${CHECK_CMD} >/dev/null 2>&1; then
  echo "Checksum verified [OK]" >&2
else
  echo "FATAL: Checksum mismatch! Expected ${EXPECTED_SHA512}" >&2
  rm -f .dapper.tmp
  exit 1
fi

chmod +x .dapper.tmp
mv .dapper.tmp .dapper
echo "--- Dapper installed successfully ---" >&2
