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

## The app shows connection errors, what do I do?

Check the following:

1. **PAT validity**: Your token may have expired. Run `azdo auth` to set a new one.
2. **PAT scopes**: Ensure your token has the required scopes (Build Read, Code Read & Write, Work Items Read & Write).
3. **Organization/project names**: Verify the `organization` and `projects` values in your config file match your Azure DevOps setup exactly.
4. **Network access**: Ensure you can reach `dev.azure.com` from your terminal. If you're behind a proxy or VPN, make sure it allows HTTPS traffic to Azure DevOps.

The app will automatically retry failed requests and show the connection status in the footer.
