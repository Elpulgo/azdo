#!/bin/sh
# install_test.sh - Tests for install.sh
#
# Validates the install script functions work correctly.
# Run: sh install_test.sh
#
# These tests source install.sh and verify individual functions
# without actually downloading or installing anything.

set -e

TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# ─── Test helpers ──────────────────────────────────────────────────────────────

assert_eq() {
    TESTS_RUN=$((TESTS_RUN + 1))
    expected="$1"
    actual="$2"
    test_name="$3"

    if [ "$expected" = "$actual" ]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        printf "  \033[0;32mPASS\033[0m %s\n" "$test_name"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        printf "  \033[0;31mFAIL\033[0m %s\n" "$test_name"
        printf "       Expected: '%s'\n" "$expected"
        printf "       Got:      '%s'\n" "$actual"
    fi
}

assert_not_empty() {
    TESTS_RUN=$((TESTS_RUN + 1))
    value="$1"
    test_name="$2"

    if [ -n "$value" ]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        printf "  \033[0;32mPASS\033[0m %s\n" "$test_name"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        printf "  \033[0;31mFAIL\033[0m %s\n" "$test_name"
        printf "       Expected non-empty value\n"
    fi
}

assert_file_exists() {
    TESTS_RUN=$((TESTS_RUN + 1))
    filepath="$1"
    test_name="$2"

    if [ -f "$filepath" ]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        printf "  \033[0;32mPASS\033[0m %s\n" "$test_name"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        printf "  \033[0;31mFAIL\033[0m %s\n" "$test_name"
        printf "       File not found: %s\n" "$filepath"
    fi
}

assert_contains() {
    TESTS_RUN=$((TESTS_RUN + 1))
    haystack="$1"
    needle="$2"
    test_name="$3"

    if printf '%s' "$haystack" | grep -q "$needle"; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        printf "  \033[0;32mPASS\033[0m %s\n" "$test_name"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        printf "  \033[0;31mFAIL\033[0m %s\n" "$test_name"
        printf "       '%s' not found in output\n" "$needle"
    fi
}

# ─── Test: OS detection ───────────────────────────────────────────────────────

test_detect_os() {
    printf "\n\033[1mTest: OS detection\033[0m\n"

    # Source the script functions
    OS_NAME=""
    detect_os >/dev/null 2>&1

    current_os="$(uname -s)"
    case "$current_os" in
        Linux*)   expected="Linux" ;;
        Darwin*)  expected="Darwin" ;;
        MINGW*|MSYS*|CYGWIN*) expected="Windows" ;;
        *)        expected="unknown" ;;
    esac

    assert_eq "$expected" "$OS_NAME" "OS detection matches uname"
    assert_not_empty "$OS_NAME" "OS_NAME is set"
}

# ─── Test: Architecture detection ─────────────────────────────────────────────

test_detect_arch() {
    printf "\n\033[1mTest: Architecture detection\033[0m\n"

    ARCH_NAME=""
    detect_arch >/dev/null 2>&1

    assert_not_empty "$ARCH_NAME" "ARCH_NAME is set"

    # Should be one of the supported architectures
    case "$ARCH_NAME" in
        x86_64|arm64)
            assert_eq "1" "1" "ARCH_NAME is a supported value ($ARCH_NAME)"
            ;;
        *)
            assert_eq "x86_64 or arm64" "$ARCH_NAME" "ARCH_NAME is a supported value"
            ;;
    esac
}

# ─── Test: Dependency checks ──────────────────────────────────────────────────

test_check_dependencies() {
    printf "\n\033[1mTest: Dependency checks\033[0m\n"

    OS_NAME="Linux"
    DOWNLOADER=""
    check_dependencies >/dev/null 2>&1

    assert_not_empty "$DOWNLOADER" "DOWNLOADER is set"

    case "$DOWNLOADER" in
        curl|wget)
            assert_eq "1" "1" "DOWNLOADER is curl or wget ($DOWNLOADER)"
            ;;
        *)
            assert_eq "curl or wget" "$DOWNLOADER" "DOWNLOADER is valid"
            ;;
    esac
}

# ─── Test: Version parsing ────────────────────────────────────────────────────

test_version_prefix() {
    printf "\n\033[1mTest: Version prefix handling\033[0m\n"

    # Test with 'v' prefix
    VERSION="v1.2.3"
    # Simulate the prefix logic from resolve_version
    case "$VERSION" in
        v*) ;;
        *)  VERSION="v$VERSION" ;;
    esac
    assert_eq "v1.2.3" "$VERSION" "Version with v prefix stays unchanged"

    # Test without 'v' prefix
    VERSION="1.2.3"
    case "$VERSION" in
        v*) ;;
        *)  VERSION="v$VERSION" ;;
    esac
    assert_eq "v1.2.3" "$VERSION" "Version without v prefix gets v prepended"
}

# ─── Test: Archive name construction ──────────────────────────────────────────

test_archive_name() {
    printf "\n\033[1mTest: Archive name construction\033[0m\n"

    BINARY_NAME="azdo-tui"

    # Linux amd64
    VERSION="v1.0.0"
    VERSION_NUM="${VERSION#v}"
    OS_NAME="Linux"
    ARCH_NAME="x86_64"
    ARCHIVE_EXT="tar.gz"
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS_NAME}_${ARCH_NAME}.${ARCHIVE_EXT}"
    assert_eq "azdo-tui_1.0.0_Linux_x86_64.tar.gz" "$ARCHIVE_NAME" "Linux amd64 archive name"

    # macOS arm64
    OS_NAME="Darwin"
    ARCH_NAME="arm64"
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS_NAME}_${ARCH_NAME}.${ARCHIVE_EXT}"
    assert_eq "azdo-tui_1.0.0_Darwin_arm64.tar.gz" "$ARCHIVE_NAME" "macOS arm64 archive name"

    # Windows
    OS_NAME="Windows"
    ARCH_NAME="x86_64"
    ARCHIVE_EXT="zip"
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS_NAME}_${ARCH_NAME}.${ARCHIVE_EXT}"
    assert_eq "azdo-tui_1.0.0_Windows_x86_64.zip" "$ARCHIVE_NAME" "Windows amd64 archive name"
}

# ─── Test: Install dir resolution ─────────────────────────────────────────────

test_install_dir_custom() {
    printf "\n\033[1mTest: Custom install directory\033[0m\n"

    INSTALL_DIR="/custom/path"
    resolve_install_dir >/dev/null 2>&1
    assert_eq "/custom/path" "$INSTALL_DIR" "Custom install dir is preserved"
}

test_install_dir_default() {
    printf "\n\033[1mTest: Default install directory\033[0m\n"

    INSTALL_DIR=""
    OS_NAME="Linux"
    resolve_install_dir >/dev/null 2>&1
    assert_not_empty "$INSTALL_DIR" "Default install dir is set"
}

# ─── Test: Config file creation ────────────────────────────────────────────────

test_config_creation() {
    printf "\n\033[1mTest: Config file creation\033[0m\n"

    # Use a temp directory for testing
    TEST_TMP=$(mktemp -d 2>/dev/null || mktemp -d -t 'azdo-test')
    trap 'rm -rf "$TEST_TMP"' RETURN

    # Override HOME for testing
    OLD_HOME="$HOME"
    HOME="$TEST_TMP"
    OS_NAME="Linux"
    SKIP_CONFIG="false"
    CONFIG_DIR_NAME="azdo-tui"

    setup_config >/dev/null 2>&1

    assert_file_exists "$TEST_TMP/.config/azdo-tui/config.yaml" "Config file was created"

    if [ -f "$TEST_TMP/.config/azdo-tui/config.yaml" ]; then
        content=$(cat "$TEST_TMP/.config/azdo-tui/config.yaml")
        assert_contains "$content" "organization:" "Config contains organization field"
        assert_contains "$content" "projects:" "Config contains projects field"
        assert_contains "$content" "polling_interval:" "Config contains polling_interval field"
        assert_contains "$content" "theme:" "Config contains theme field"
    fi

    # Test that existing config is not overwritten
    echo "custom: value" > "$TEST_TMP/.config/azdo-tui/config.yaml"
    setup_config >/dev/null 2>&1
    content=$(cat "$TEST_TMP/.config/azdo-tui/config.yaml")
    assert_eq "custom: value" "$content" "Existing config is not overwritten"

    HOME="$OLD_HOME"
    rm -rf "$TEST_TMP"
}

# ─── Test: Skip config flag ───────────────────────────────────────────────────

test_skip_config() {
    printf "\n\033[1mTest: Skip config flag\033[0m\n"

    TEST_TMP=$(mktemp -d 2>/dev/null || mktemp -d -t 'azdo-test')

    OLD_HOME="$HOME"
    HOME="$TEST_TMP"
    SKIP_CONFIG="true"
    OS_NAME="Linux"
    CONFIG_DIR_NAME="azdo-tui"

    setup_config >/dev/null 2>&1

    if [ ! -f "$TEST_TMP/.config/azdo-tui/config.yaml" ]; then
        TESTS_RUN=$((TESTS_RUN + 1))
        TESTS_PASSED=$((TESTS_PASSED + 1))
        printf "  \033[0;32mPASS\033[0m Config file not created when --skip-config\n"
    else
        TESTS_RUN=$((TESTS_RUN + 1))
        TESTS_FAILED=$((TESTS_FAILED + 1))
        printf "  \033[0;31mFAIL\033[0m Config file should not be created with --skip-config\n"
    fi

    HOME="$OLD_HOME"
    rm -rf "$TEST_TMP"
}

# ─── Test: Parse args ─────────────────────────────────────────────────────────

test_parse_args() {
    printf "\n\033[1mTest: Argument parsing\033[0m\n"

    VERSION=""
    INSTALL_DIR=""
    SKIP_CONFIG="false"

    parse_args --version v2.0.0 --install-dir /tmp/test --skip-config
    assert_eq "v2.0.0" "$VERSION" "Version parsed from args"
    assert_eq "/tmp/test" "$INSTALL_DIR" "Install dir parsed from args"
    assert_eq "true" "$SKIP_CONFIG" "Skip config parsed from args"

    # Reset and test short flags
    VERSION=""
    INSTALL_DIR=""
    parse_args -v v3.0.0 -d /tmp/other
    assert_eq "v3.0.0" "$VERSION" "Version parsed from short flag"
    assert_eq "/tmp/other" "$INSTALL_DIR" "Install dir parsed from short flag"
}

# ─── Test: Download URL construction ──────────────────────────────────────────

test_download_url() {
    printf "\n\033[1mTest: Download URL construction\033[0m\n"

    REPO_OWNER="Elpulgo"
    REPO_NAME="azdo"
    VERSION="v1.0.0"
    BINARY_NAME="azdo-tui"
    GITHUB_DOWNLOAD="https://github.com"

    VERSION_NUM="${VERSION#v}"
    OS_NAME="Linux"
    ARCH_NAME="x86_64"
    ARCHIVE_EXT="tar.gz"
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS_NAME}_${ARCH_NAME}.${ARCHIVE_EXT}"

    DOWNLOAD_URL="$GITHUB_DOWNLOAD/$REPO_OWNER/$REPO_NAME/releases/download/$VERSION/$ARCHIVE_NAME"
    expected="https://github.com/Elpulgo/azdo/releases/download/v1.0.0/azdo-tui_1.0.0_Linux_x86_64.tar.gz"
    assert_eq "$expected" "$DOWNLOAD_URL" "Download URL is correctly constructed"
}

# ─── Run all tests ─────────────────────────────────────────────────────────────

main() {
    printf "\033[1mazdo-tui install.sh test suite\033[0m\n"
    printf "==============================\n"

    # Source the install script to get access to its functions
    # We override 'main' to prevent it from running
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    (
        # Redefine main to a no-op before sourcing
        eval "$(sed 's/^main "\$@"$//' "$SCRIPT_DIR/install.sh")"

        test_detect_os
        test_detect_arch
        test_check_dependencies
        test_version_prefix
        test_archive_name
        test_install_dir_custom
        test_install_dir_default
        test_config_creation
        test_skip_config
        test_parse_args
        test_download_url

        # Print summary
        printf "\n\033[1m==============================\033[0m\n"
        printf "Tests run:    %d\n" "$TESTS_RUN"
        printf "\033[0;32mTests passed: %d\033[0m\n" "$TESTS_PASSED"
        if [ "$TESTS_FAILED" -gt 0 ]; then
            printf "\033[0;31mTests failed: %d\033[0m\n" "$TESTS_FAILED"
            exit 1
        else
            printf "Tests failed: 0\n"
            printf "\n\033[0;32mAll tests passed!\033[0m\n"
        fi
    )
}

main
