VAGRANTFILE_API_VERSION = "2"

require './vagrant-common.rb'

vm_ip = "172.16.0.3" # arbitrary private IP

pkgs = %w(
  docker-engine
  aufs-tools
  build-essential
  ethtool
  iputils-arping
  libpcap-dev
  git
  mercurial
  bc
)

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|

  config.vm.box = "ubuntu/ubuntu-15.04-amd64"
  config.vm.box_url = "https://cloud-images.ubuntu.com/vagrant/vivid/current/vivid-server-cloudimg-amd64-vagrant-disk1.box"

  config.vm.network "private_network", ip: vm_ip
  config.vm.provider :virtualbox do |vb|
    vb.customize ["modifyvm", :id, "--natdnshostresolver1", "off"]
    vb.customize ["modifyvm", :id, "--natdnsproxy1", "off"]
  end

  config.vm.synced_folder ".", "/vagrant", disabled: true
  config.vm.synced_folder ".", "/home/vagrant/src/github.com/weaveworks/weave"

  install_build_deps config.vm, pkgs
  install_go_toochain config.vm
  tweak_user_env config.vm
  tweak_docker_daemon config.vm
  cleanup config.vm

end

begin
  load 'Vagrantfile.local'
rescue LoadError
end
