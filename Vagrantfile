require File.expand_path(File.join(File.dirname(__FILE__), 'vagrant-common.rb'))

vm_ip = "172.16.0.3" # arbitrary private IP

pkgs = %w(
  docker-engine=1.10.2-0~wily
  aufs-tools
  build-essential
  ethtool
  iputils-arping
  libpcap-dev
  git
  mercurial
  bc
  jq
)

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|

  config.vm.box = VAGRANT_IMAGE

  config.vm.network "private_network", ip: vm_ip
  config.vm.provider :virtualbox do |vb|
    vb.memory = 2048
    configure_nat_dns(vb)    
  end

  # Disable default Vagrant shared folder, which we don't need:
  config.vm.synced_folder ".", "/vagrant", disabled: true
  # Keep Weave Net sources' in sync:
  config.vm.synced_folder ".", "/home/vagrant/src/github.com/weaveworks/weave"
  # Create a convenience symlink to $HOME/src/github.com/weaveworks/weave
  config.vm.provision :shell, :inline => 'ln -sf ~vagrant/src/github.com/weaveworks/weave ~vagrant/'

  # Set SSH keys up to be able to run smoke tests straightaway:
  config.vm.provision "file", source: "~/.vagrant.d/insecure_private_key", destination: "/home/vagrant/src/github.com/weaveworks/weave/test/insecure_private_key"

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
