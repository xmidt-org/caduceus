%define debug_package %{nil}

Name:       caduceus
Version:    %{_ver}
Release:    %{_releaseno}%{?dist}
Summary:    The Xmidt API interface server.

Group:      System Environment/Daemons
License:    ASL 2.0
URL:        https://github.com/Comcast/%{name}
Source0:    %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.11

Provides:       %{name}

%description
The Xmidt API interface server.

%prep
%setup -q

%build
export GOPATH=$(pwd)
pushd src
glide i --strip-vendor
cd %{name}
go build %{name}
popd

%install

# Install Binary
%{__install} -D -p -m 755 src/%{name}/%{name} %{buildroot}%{_bindir}/%{name}

# Install Service
%{__install} -D -p -m 644 etc/systemd/%{name}.service %{buildroot}%{_unitdir}/%{name}.service

# Install Configuration
%{__install} -D -p -m 644 etc/%{name}/%{name}.yaml %{buildroot}%{_sysconfdir}/%{name}/%{name}.yaml


# Create Logging Location
%{__install} -d %{buildroot}%{_localstatedir}/log/%{name}

# Create Runtime Details Location
%{__install} -d %{buildroot}%{_localstatedir}/run/%{name}

%files
%defattr(-, %{name}, %{name}, -)

# Binary
%attr(755, %{name}, %{name}) %{_bindir}/%{name} 

# Configuration
%dir %{_sysconfdir}/%{name}
%config(noreplace) %{_sysconfdir}/%{name}/%{name}.yaml

# Service Files
%{_unitdir}/%{name}.service

# Logging Location
%dir %{_localstatedir}/log/%{name}

# Runtime Details Location
%dir %{_localstatedir}/run/%{name}

%pre
# If app user does not exist, create
id %{name} >/dev/null 2>&1
if [ $? != 0 ]; then
    /usr/sbin/groupadd -r %{name} >/dev/null 2>&1
    /usr/sbin/useradd -d /var/run/%{name} -r -g %{name} %{name} >/dev/null 2>&1
fi


%post
%systemd_post %{name}.service

%preun
%systemd_preun %{name}.service

%postun
%systemd_postun %{name}.service

# Do not remove anything if this is not an uninstall
if [ $1 = 0 ]; then
    /usr/sbin/userdel -r %{name} >/dev/null 2>&1
    /usr/sbin/groupdel %{name} >/dev/null 2>&1
    # Ignore errors from above
    true
fi

%changelog
