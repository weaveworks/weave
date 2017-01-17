VAGRANT_IMAGE = 'ubuntu/wily64'
VAGRANTFILE_API_VERSION = '2'

def get_go_version_from_build_dockerfile()
  go_regexp = /FROM golang:(\S*).*?/
  dockerfile_path = File.expand_path(File.join(File.dirname(__FILE__), 'build', 'Dockerfile'))
  go_version = File.readlines(dockerfile_path).first { |line| line.match(go_regexp) }.match(go_regexp).captures.first
  if go_version.nil?
    raise ArgumentError.new("Failed to read Go version from Dockerfile.")
  end
  go_version
end

GO_BINARY_PATH = '/usr/local/go/bin'
GO_VERSION = get_go_version_from_build_dockerfile()

def configure_nat_dns(vb)
  vb.customize ["modifyvm", :id, "--natdnshostresolver1", "off"]
  vb.customize ["modifyvm", :id, "--natdnsproxy1", "off"]
end

def install_packages(vm, pkgs)
  vm.provision :shell, :inline => <<SCRIPT
apt-get update -qq
apt-get install -qq -y --force-yes --no-install-recommends #{pkgs.join(' ')}
SCRIPT
end

def install_build_deps(vm, pkgs)
  vm.provision :shell, :inline => <<SCRIPT
export DEBIAN_FRONTEND=noninteractive
apt-key adv \
  --keyserver hkp://p80.pool.sks-keyservers.net:80 \
  --recv-keys 58118E89F3A912897C070ADBF76221572C52609D
echo 'deb https://apt.dockerproject.org/repo ubuntu-wily main' \
  > /etc/apt/sources.list.d/docker.list
SCRIPT
  install_packages(vm, pkgs)
end

def install_go_toochain(vm)
  vm.provision :shell, :inline => <<SCRIPT
curl -s https://storage.googleapis.com/golang/go#{GO_VERSION}.linux-amd64.tar.gz \
  | tar xz -C /usr/local
#{GO_BINARY_PATH}/go clean -i net
#{GO_BINARY_PATH}/go install -tags netgo std
SCRIPT
end

def tweak_user_env(vm)
  script = <<SCRIPT
echo 'export GOPATH="${HOME}"' \
  >> ~vagrant/.profile
echo 'export PATH="${HOME}/bin:#{GO_BINARY_PATH}:${PATH}"' \
  >> ~vagrant/.profile
ln -sf ~vagrant/src/github.com/weaveworks/weave ~vagrant/
sudo chown -R vagrant:vagrant ~vagrant/src
SCRIPT
  vm.provision :shell, :inline => script, :privileged => false
end

def tweak_docker_daemon(vm)
  vm.provision :shell, :inline => <<SCRIPT
usermod -a -G docker vagrant
mkdir -p /etc/systemd/system/docker.service.d
cat >/etc/systemd/system/docker.service.d/override.conf  <<EOF
[Service]
ExecStart=
ExecStart=/usr/bin/docker daemon -H fd:// -H unix:///var/run/alt-docker.sock -H tcp://0.0.0.0:2375 -s overlay
EOF
systemctl daemon-reload
systemctl restart docker
systemctl enable docker
SCRIPT
end

def cleanup(vm)
  vm.provision :shell, :inline => <<SCRIPT
export DEBIAN_FRONTEND=noninteractive
## Who the hell thinks official images have to have both of these?
/etc/init.d/chef-client stop
/etc/init.d/puppet stop
apt-get -qq remove puppet chef
apt-get -qq autoremove
killall -9 chef-client 2>/dev/null || true
SCRIPT
end
