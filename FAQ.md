# FAQ

## How do I update my PAT?

Run `azdo auth` from the command line. This will prompt you to enter a new Personal Access Token which replaces the existing one stored in your system keyring.

## Where is the config file?

The configuration file is located at:
- **Linux/macOS**: `~/.config/azdo-tui/config.yaml`
- **Windows**: `C:\Users\<username>\.config\azdo-tui\config.yaml`

If no config file exists, the setup wizard will guide you through creating one on first launch.

## What PAT scopes do I need?

| Scope | Access | Used For |
|-------|--------|----------|
| **Build** | Read | Pipeline runs, build timelines, and logs |
| **Code** | Read & Write | List PRs, view threads/iterations/diffs, vote on PRs, add comments, and update thread status |
| **Work Items** | Read & Write | Query and view work items, fetch available states, and change work item state |

Create a PAT at: Azure DevOps → User Settings → Personal Access Tokens.

## Can I use an environment variable instead of the keyring?

Yes. Set the `AZDO_PAT` environment variable with your token. This is used as a fallback when the system keyring is not available or not desired.

```bash
export AZDO_PAT="your-pat-here"
azdo
```

## How do I create a custom theme?

1. Create a JSON file in the themes directory:
   - **Linux/macOS**: `~/.config/azdo-tui/themes/mytheme.json`
   - **Windows**: `C:\Users\<username>\.config\azdo-tui\themes\mytheme.json`

2. Define your colors (see `example-theme.json` in the repo for all available properties).

3. Set `theme: mytheme` in your `config.yaml`.

You can also switch themes at runtime by pressing `t` to open the theme picker.

## What platforms are supported?

| Platform | Architecture |
|----------|-------------|
| Linux    | x86_64, ARM64 |
| macOS    | x86_64, ARM64 (Apple Silicon) |
| Windows  | x86_64, ARM64 |

Pre-built binaries for all platforms are available on the [Releases page](https://github.com/Elpulgo/azdo/releases).

## How do I filter work items to just mine?

Press `m` in the Work Items tab to toggle the "my items" filter. This shows only work items assigned to you.

You can also press `f` to open the search/filter bar and type to filter by text, or press `T` to filter by tag.

## Where is the state file and how do I reset it?

The app persists a small amount of navigation state (last active tab, last opened PR / work item detail) so you resume where you left off. The file lives at:

- **Linux/macOS**: `$XDG_STATE_HOME/azdo-tui/state.yaml` if set, otherwise `~/.local/state/azdo-tui/state.yaml`
- **Windows**: `%USERPROFILE%\.local\state\azdo-tui\state.yaml`

It is created lazily on first save — a missing file is normal and not an error. Delete it to reset to the default view (Pull Requests tab, no detail open). Pipeline detail is intentionally not persisted.

## I don't see a Notifications tab

The Notifications tab renders your GitHub inbox, so it only appears when at least one
configured backend supports it — in the current release that means **GitHub only**. If
your config is Azure-only (no `github.repos`), there is nothing to show the tab for and it
is hidden entirely; that's expected, not a bug, and Azure DevOps support is planned for a
later release. Once it lands, the tab reappears on its own — no config change needed.

If you do have `github.repos` configured and still don't see it, check `disabled_panes` in
your config file for `notifications`, and check that your GitHub token has the required
scope (see the next question).

## The Notifications tab says my token is missing a scope

The GitHub token needs an extra scope beyond what the rest of the app uses: `notifications`
on a classic PAT, or the account-level **Notifications** permission on a fine-grained
token. This is a common situation if you set up your token before the Notifications tab
existed — the fix is to add the scope to your existing token (or generate a new
fine-grained one with the permission) and run `azdo auth` to update the stored token. See
[GitHub — Personal Access Token](README.md#github--personal-access-token) in the README for
exactly where to add it.

If you'd rather not touch your token, add `notifications` to `disabled_panes` in your
config instead — or, from the pane's own error screen, press `x` twice, which writes that
for you. Either way it takes effect on the next restart, not immediately.

One thing this is **not**: if you're seeing an ordinary "unavailable" message with no
mention of a scope, that's most likely a rate limit, not a missing permission — adding a
scope to an already-correct token won't fix that.

## How do I disable the Notifications tab?

Add `notifications` to `disabled_panes` in your config file (comma-separated with any other
panes you've already disabled), or, if the pane is showing an error, press `x` twice from
within it — that writes `disabled_panes` for you. Either way the change takes effect on the
**next restart**; the tab list is computed once at startup, so it won't vanish mid-session.

## The app shows connection errors, what do I do?

Check the following:

1. **PAT validity**: Your token may have expired. Run `azdo auth` to set a new one.
2. **PAT scopes**: Ensure your token has the required scopes (Build Read, Code Read & Write, Work Items Read & Write).
3. **Organization/project names**: Verify the `organization` and `projects` values in your config file match your Azure DevOps setup exactly.
4. **Network access**: Ensure you can reach `dev.azure.com` from your terminal. If you're behind a proxy or VPN, make sure it allows HTTPS traffic to Azure DevOps.

The app will automatically retry failed requests and show the connection status in the footer.
