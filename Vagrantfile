VAGRANTFILE_API_VERSION = "2"

vm_ip = "172.16.0.3" # arbitrary private IP
pkgs = "lxc-docker aufs-tools build-essential ethtool libpcap-dev git mercurial bc"

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|

  config.vm.box = "phusion/ubuntu-14.04-amd64"
  config.vm.box_url = "https://oss-binaries.phusionpassenger.com/vagrant/boxes/latest/ubuntu-14.04-amd64-vbox.box"

  config.vm.network "private_network", ip: vm_ip
  config.vm.provider :virtualbox do |vb|
    vb.customize ["modifyvm", :id, "--natdnshostresolver1", "off"]
    vb.customize ["modifyvm", :id, "--natdnsproxy1", "off"]
  end

  config.vm.synced_folder ".", "/vagrant", disabled: true
  config.vm.synced_folder ".", "/home/vagrant/src/github.com/zettio/weave"
  config.vm.provision :shell, :inline => "ln -s /home/vagrant/src/github.com/zettio/weave /home/vagrant/weave"

  config.vm.provision :shell, :inline => "sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9"
  config.vm.provision :shell, :inline => "echo deb https://get.docker.io/ubuntu docker main > /etc/apt/sources.list.d/docker.list"

  pkg_cmd = "export DEBIAN_FRONTEND=noninteractive; " \
    "apt-get update -qq; " \
    "apt-get install -qq --no-install-recommends #{pkgs}"

  install_go_toolchain = "curl -s https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz | tar xz -C /usr/local"

  config.vm.provision :shell, :inline => pkg_cmd
  config.vm.provision :shell, :inline => install_go_toolchain
  config.vm.provision :shell, :inline => "usermod -a -G docker vagrant; "
  config.vm.provision :shell, :inline => "echo export GOPATH=/home/vagrant >> /home/vagrant/.bashrc"
  config.vm.provision :shell, :inline => "echo export PATH=/usr/local/go/bin:${PATH} >> /home/vagrant/.bashrc"
  config.vm.provision :shell, :inline => "chown -R vagrant:vagrant /home/vagrant/src"
  config.vm.provision :shell, :inline => "/usr/local/go/bin/go clean -i net; /usr/local/go/bin/go install -tags netgo std"

  config.vm.provision :shell, :inline => "echo 'DOCKER_OPTS=\"-H unix:///var/run/docker.sock -H tcp://0.0.0.0:2375\"' >> /etc/default/docker"
  config.vm.provision :shell, :inline => "service docker restart"

end

begin
  load 'Vagrantfile.local'
rescue LoadError
end
