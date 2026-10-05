# Installation

To get started, install Docker, then install Taxiway. Add Lima when you want
VM-backed labs.

## Prerequisites

- **Docker**, running on your machine, with Docker Compose, for either driver.
- **Lima**, when using the Lima driver; Docker-only labs do not require it.

Already have the prerequisites for your driver? Go straight to
[Install Taxiway](#install-taxiway).

### macOS

Install and open [Docker Desktop](https://docs.docker.com/desktop/setup/install/mac-install/).
For Lima-backed labs, also follow the
[Lima installation guide](https://lima-vm.io/docs/installation/).

### Linux

Follow the [Docker Engine installation guide](https://docs.docker.com/engine/install/)
for your distribution, including the Compose plugin. For Lima-backed labs,
also follow the [Lima installation guide](https://lima-vm.io/docs/installation/),
including QEMU.

### Windows with WSL2

Set up [WSL2](https://learn.microsoft.com/en-us/windows/wsl/install) with your
preferred Linux distribution. Inside that distribution, follow the Linux
instructions above to install Docker and Lima.

Run all the commands below in your WSL2 terminal.

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
(`~/.zshrc` for Zsh or `~/.bashrc` for Bash), then reopen the terminal:

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
