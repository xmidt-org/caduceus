%define AutoReqProv: no
%define __os_install_post %{nil}

Name:       caduceus
Version:    %{_ver}
Release:    %{_releaseno}%{?dist}
Summary:    The Xmidt API interface server.

Vendor:     Comcast
Packager:   Comcast
Group:      System Environment/Daemons
License:    ASL 2.0
URL:        https://github.com/xmidt-org/caduceus
Source0:    %{name}-%{_version}.tar.gz

Prefix:     /opt
BuildRoot:  %{_tmppath}/%{name}
BuildRequires: systemd
BuildRequires: golang >= 1.11

%description
The shield to protect our users from incoming events for the codex project.
aka. The receiver of data from XMiDT Caduceus

%build
GO111MODULE=on go build -o $RPM_SOURCE_DIR/%{name} %{_topdir}/..

%install
echo rm -rf %{buildroot}
%{__install} -d %{buildroot}%{_bindir}
%{__install} -d %{buildroot}%{_initddir}
%{__install} -d %{buildroot}%{_sysconfdir}/%{name}
%{__install} -d %{buildroot}%{_localstatedir}/log/%{name}
%{__install} -d %{buildroot}%{_localstatedir}/run/%{name}
%{__install} -d %{buildroot}%{_unitdir}

%{__install} -p $RPM_SOURCE_DIR/%{name} %{buildroot}%{_bindir}
%{__install} -p $RPM_SOURCE_DIR/%{name}.service %{buildroot}%{_unitdir}/%{name}.service
%{__install} -p $RPM_SOURCE_DIR/%{name}.yaml %{buildroot}%{_sysconfdir}/%{name}/%{name}.yaml

%files
%defattr(644, root, root, 755)
%doc $RPM_SOURCE_DIR/LICENSE $RPM_SOURCE_DIR/CHANGELOG.md $RPM_SOURCE_DIR/NOTICE

%attr(755, root, root) %{_bindir}/%{name}

%{_unitdir}/%{name}.service

%dir %{_sysconfdir}/%{name}
%config %{_sysconfdir}/%{name}/%{name}.yaml

%dir %attr(755, %{name}, %{name}) %{_localstatedir}/log/%{name}
%dir %attr(755, %{name}, %{name}) %{_localstatedir}/run/%{name}

%pre
id %{name} >/dev/null 2>&1
if [ $? != 0 ]; then
    /usr/sbin/groupadd -r %{name} >/dev/null 2>&1
    /usr/sbin/useradd -d /var/run/%{name} -r -g %{name} %{name} >/dev/null 2>&1
fi

%post
if [ $1 = 1 ]; then
    systemctl preset %{name}.service >/dev/null 2>&1 || :
fi

%preun
if [ -e /etc/init.d/%{name} ]; then
    systemctl --no-reload disable %{name}.service > /dev/null 2>&1 || :
    systemctl stop %{name}.service > /dev/null 2>&1 || :
fi

# If not an upgrade, then delete
if [ $1 = 0 ]; then
    systemctl disable %{name}.service >/dev/null 2>&1 || :
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
