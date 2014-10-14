VAGRANTFILE_API_VERSION = "2"

vm_ip = "172.16.0.3" # arbitrary private IP
pkgs = "docker.io golang ethtool libpcap-dev git mercurial"

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.vm.box = "phusion/ubuntu-14.04-amd64"
  config.vm.box_url = "https://oss-binaries.phusionpassenger.com/vagrant/boxes/latest/ubuntu-14.04-amd64-vbox.box"
  
  config.vm.network "private_network", ip: vm_ip

  config.vm.synced_folder "./", "/home/vagrant/src/github.com/zettio/weave"

  pkg_cmd = "apt-get update -qq; " \
  "apt-get install -q -y --force-yes --no-install-recommends "
  pkg_cmd << pkgs
  config.vm.provision :shell, :inline => pkg_cmd
  config.vm.provision :shell, :inline => "usermod -a -G docker vagrant; "
  config.vm.provision :shell, :inline => "echo export GOPATH=/home/vagrant >> /home/vagrant/.bashrc"
  config.vm.provision :shell, :inline => "chown -R vagrant:vagrant /home/vagrant/src"
  config.vm.provision :shell, :inline => "sudo go install -a -tags netgo std"

end

begin
  load 'Vagrantfile.local'
rescue LoadError
end
