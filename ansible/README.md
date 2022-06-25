# oSSH Ansible
**WARNING**: Running this playbook on an existing host might cause errors that can lock you out of that machine. It is recommended that you use machines that are solely intended to run oSSH. The playbook should work out of the box with Digital Ocean droplets using Ubuntu 20.04 (LTS) x64 as OS, other machines might need more advanced configuration (see examples in `inventory.yml.example`).

## Preparation
You can use this playbook to deploy one or more instances of oSSH. Before you can use it, you need to do the following:  
```bash
cp ansible.cfg.example ansible.cfg
cp inventory.yml.example inventory.yml
nano inventory.yml
```

Add your oSSH hosts to `inventory.yml` and adjust everything else to your liking. The default settings should be fine for almost everybody. 

If your nodes are behind a firewall you have to open these ports:
- `admin_port` (default: 1984)
- `ui_port` (default: 443)
- `sync_port` (default: 1337)

## Installation
Once you've setup your `inventory.yml` you are ready to install the oSSH nodes. Go to the root directory of the repo and run `./ansible-install`.
This will prepare your local machine and do the initial install of all nodes. Once a host has been installed you don't need to run this again. If you want to add a new node to your cluster, you can use `./ansible-install -l 127.0.0.1,my-new-node` to run the initial install on the new node, where `my-new-node` is the host name in `inventory.yml`. The `127.0.0.1` is needed, so the playbook can build the latest oSSH version before installing the host.

## Updating
You can also use this playbook to update your nodes. To do so, go to the root directory of the repo and run `./ansible-update`. If you only want to update a single host, run `./ansible-update -l 127.0.0.1,my-host` where `my-host` is the host name in the `inventory.yml` file. The `127.0.0.1` is needed, so the playbook can build the latest oSSH version before updating the host.

## Behind the scenes ...
... the playbook will do a bunch of things for you, here's what it does:

1. (install, update) Build latest oSSH version locally and store it in `ansible/roles/ossh/files`
2. (install) Generate SSH key to use for connecting to the nodes
3. (install) Install dependencies via `apt` (acl, gnupg, sudo, nano, mc, ufw, tree)
4. (install) Create admin user and group
5. (install) Setup passwordless sudo for admin user
6. (install) Install SSH key for admin user
7. (install) Setup shell for admin and root user
8. (install) Clear root password
9. (install) Create sshd config and reload sshd
10. (install, update) Whitelist IPs for real SSH access
11. (install, update) Open required ports for oSSH
12. (install) Allow all outgoing traffic, deny all incoming traffic
13. (install) Enable ufw
14. (install, update) Set hostname to match `inventory.yml`
15. (install) Create directory for oSSH
16. (install, update) Generate oSSH config
17. (install, update) Upload oSSH binary built in step 1
18. (install, update) Install/update the oSSH systemd service
19. (install, update) Reload systemd services
20. (install) Enable oSSH systemd service
21. (update) Stop oSSH systemd service
22. (install, update) Install oSSH binary
23. (install, update) Reload oSSH service