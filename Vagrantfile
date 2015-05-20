VAGRANTFILE_API_VERSION = "2"

vm_ip = "172.16.0.3" # arbitrary private IP

go_version = "1.4.2"

pkgs = %w(
  lxc-docker
  aufs-tools
  build-essential
  ethtool
  iputils-arping
  libpcap-dev
  git
  mercurial
  bc
)

go_path = "/usr/local/go/bin"

$install_build_deps = <<SCRIPT
export DEBIAN_FRONTEND=noninteractive
apt-key adv \
  --keyserver hkp://keyserver.ubuntu.com:80 \
  --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9
echo 'deb https://get.docker.io/ubuntu docker main' \
  > /etc/apt/sources.list.d/docker.list
apt-get -qq update
apt-get -qq install --no-install-recommends #{pkgs.join(' ')}
SCRIPT

$install_go_toochain = <<SCRIPT
curl -s https://storage.googleapis.com/golang/go#{go_version}.linux-amd64.tar.gz \
  | tar xz -C /usr/local
#{go_path}/go clean -i net
#{go_path}/go install -tags netgo std
SCRIPT

$tweak_user_env = <<SCRIPT
echo 'export GOPATH="${HOME}"' \
  >> ~vagrant/.profile
echo 'export PATH="${HOME}/bin:#{go_path}:${PATH}"' \
  >> ~vagrant/.profile
ln -sf ~vagrant/src/github.com/weaveworks/weave ~vagrant/
sudo chown -R vagrant:vagrant ~vagrant/src
SCRIPT

$tweak_docker_daemon = <<SCRIPT
usermod -a -G docker vagrant
echo 'DOCKER_OPTS="-H unix:///var/run/docker.sock -H tcp://0.0.0.0:2375"' \
  >> /etc/default/docker
service docker restart
SCRIPT

$cleanup = <<SCRIPT
export DEBIAN_FRONTEND=noninteractive
## Who the hell thinks official images have to have both of these?
/etc/init.d/chef-client stop
/etc/init.d/puppet stop
apt-get -qq remove puppet chef
apt-get -qq autoremove
killall -9 chef-client
SCRIPT

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|

  config.vm.box = "ubuntu/ubuntu-14.10-amd64"
  config.vm.box_url = "https://cloud-images.ubuntu.com/vagrant/utopic/current/utopic-server-cloudimg-amd64-vagrant-disk1.box"

  config.vm.network "private_network", ip: vm_ip
  config.vm.provider :virtualbox do |vb|
    vb.customize ["modifyvm", :id, "--natdnshostresolver1", "off"]
    vb.customize ["modifyvm", :id, "--natdnsproxy1", "off"]
  end

  config.vm.synced_folder ".", "/vagrant", disabled: true
  config.vm.synced_folder ".", "/home/vagrant/src/github.com/weaveworks/weave"

  config.vm.provision :shell, :inline => $install_build_deps
  config.vm.provision :shell, :inline => $install_go_toochain
  config.vm.provision :shell, :inline => $tweak_user_env, :privileged => false
  config.vm.provision :shell, :inline => $tweak_docker_daemon
  config.vm.provision :shell, :inline => $cleanup

end

begin
  load 'Vagrantfile.local'
rescue LoadError
end
