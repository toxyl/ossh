# oSSH Ansible

WARNING: Running this on existing hosts might cause errors that can lock you out of that machine. It is recommended that you use machines that are solely intended to run oSSH. The playbook should out of the box with Digital Ocean droplets using Ubuntu 20.04 (LTS) x64 as OS, other machines might need more advanced configuration (see examples in `inventory.yml.example`).

You can use this playbook to deploy one or more instances of oSSH. Before you can use it, you need to do the following:  
```bash
cp ansible.cfg.example ansible.cfg
cp inventory.yml.example inventory.yml
nano inventory.yml
```

Add your oSSH hosts to `inventory.yml` and adjust everything else to your liking. The default settings should be fine for almost everybody. 

Once you've setup your `inventory.yml` you are ready to install or update oSSH nodes. For your convenvience there are two scripts in the root folder of the repo. First you need to run `./ansible-install` which will prepare your local machine and install oSSH to all hosts. From then on you can just run `./ansible-update` to update your instances. If you only want to install/update a single host, run `./ansible-update -l 127.0.0.1,my-host` (or `ansible-install` to install a new host) where `my-host` is the host name in the `inventory.yml` file. The `127.0.0.1` is needed, so the playbook can build the latest oSSH version before installing/updating the host.


