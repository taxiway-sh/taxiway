# Installation

To get started, install Docker and Lima, then install Taxiway.

## Prerequisites

- **Docker**, running on your machine, with Docker Compose.
- **Lima**, to create your lab environments.

Already have both? Go straight to [Install Taxiway](#install-taxiway).

### macOS

Install and open [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/).
Then follow the [Lima installation guide](https://lima-vm.io/docs/installation/).

### Linux

Follow the [Docker Engine installation guide](https://docs.docker.com/engine/install/)
for your distribution, including the Compose plugin. Then follow the
[Lima installation guide](https://lima-vm.io/docs/installation/), including QEMU.

### Windows with WSL2

Install [Ubuntu with WSL2](https://learn.microsoft.com/en-us/windows/wsl/install).
Inside Ubuntu, follow the Linux instructions above to install Docker and Lima.

Run all the commands below in the Ubuntu terminal.

## Install Taxiway

Copy this command into your terminal:

```bash
curl -fsSL https://taxiway.run | sh
```

Then initialize Taxiway:

```bash
taxiway init
```

The first run downloads and starts the shared services. This can take a few
minutes.

If your terminal cannot find `taxiway`, add it to your shell configuration
(`~/.zshrc` on macOS or `~/.bashrc` on Ubuntu), then reopen the terminal:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## Check your installation

```bash
taxiway status
```

Check that Docker is available and the shared services are running.
You can then [choose an orchestrator](../README.md#orchestrators) and create
your first lab.

For a specific release or a different installation directory, see
[Installer options](../contributing/release.md#installer).
