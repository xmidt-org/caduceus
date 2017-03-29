%define debug_package %{nil}

Name:       caduceus
Version:    %{_ver}
Release:    %{_releaseno}%{?dist}
Summary:    The WebPA HTTP redirector component.

Group:      System Environment/Daemons
License:    ASL 2.0
URL:        https://github.com/Comcast/caduceus
Source0:    %{name}-%{_fullver}.tar.gz

BuildRequires:  golang >= 1.7
Requires:       supervisor

%description
Webpa notifications delivery server.

%prep
%setup -q


%build
export GOPATH=$(pwd)
pushd src
glide install
cd caduceus
go build %{name}
popd

%install

# Install Binary
%{__install} -d %{buildroot}%{_bindir}
%{__install} -p src/caduceus/%{name} %{buildroot}%{_bindir}

# Install Service
%{__install} -d %{buildroot}%{_initddir}
%{__install} -p etc/init.d/%{name} %{buildroot}%{_initddir}

# Install Configuration
%{__install} -d %{buildroot}%{_sysconfdir}/%{name}
%{__install} -p etc/%{name}/%{name}.cfg %{buildroot}%{_sysconfdir}/%{name}/%{name}.cfg
%{__install} -p etc/%{name}/supervisord.conf %{buildroot}%{_sysconfdir}/%{name}/supervisord.conf

# Create Logging Location
%{__install} -d %{buildroot}%{_localstatedir}/%{name}


%files
%defattr(644, caduceus, caduceus, 755)

# Binary
%dir %{_bindir}
%attr(755, caduceus, caduceus) %{_bindir}/%{name} 

# Init.d
%attr(755, caduceus, caduceus) %{_initddir}/%{name}

# Configuration
%dir %{_sysconfdir}/%{name}
%config %attr(644, caduceus, caduceus) %{_sysconfdir}/%{name}/%{name}.cfg
%config %attr(644, caduceus, caduceus) %{_sysconfdir}/%{name}/supervisord.conf

# Logging Location
%dir %{_localstatedir}/%{name}

%pre
# If app user does not exist, create
id %{name} >/dev/null 2>&1
if [ $? != 0 ]; then
    /usr/sbin/groupadd -r %{name} >/dev/null 2>&1
    /usr/sbin/useradd -d /var/run/%{name} -r -g %{name} %{name} >/dev/null 2>&1
fi


%post
if [ $1 = 1 ]; then
    /sbin/chkconfig --add %{name}
fi

%preun
# Stop service if running
if [ -e /etc/init.d/%{name} ]; then
    /sbin/service %{name} stop > /dev/null 2>&1
    true
fi

# If not an upgrade, then delete
if [ $1 = 0 ]; then
    /sbin/chkconfig --del %{name} > /dev/null 2>&1
    true
fi

%postun
# Do not remove anything if this is not an uninstall
if [ $1 = 0 ]; then
    /usr/sbin/userdel -r %{name} >/dev/null 2>&1
    /usr/sbin/groupdel %{name} >/dev/null 2>&1
    # Ignore errors from above
    true
fi

%changelog
