Name:               mongodb-mms-atlas-restore
Version:            @VERSION@
Release:            @PACKAGE_VERSION@
Summary:            MongoDB Atlas Restore Server
Group:              Application/System
License:            Restricted
URL:                https://cloud.mongodb.com
Vendor:             MongoDB Inc.
BuildArchitectures: @ARCHITECTURE@

%description
The MongoDB Atlas Restore Server

%prep
echo ${version}

%build
echo ${_sourcedir}

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p $RPM_BUILD_ROOT
cp -a %{_sourcedir}/* $RPM_BUILD_ROOT
mkdir -p $RPM_BUILD_ROOT/var/lib/mongodb-mms-atlas-restore $RPM_BUILD_ROOT/var/log/mongodb-mms-atlas-restore

%clean
rm -rf $RPM_BUILD_ROOT

%files
%attr(0755,atlas-restore,atlas-restore) /opt/mongodb-mms-atlas-restore/bin/mongodb-mms-atlas-restore
%attr(0644,root,root) /etc/systemd/system/mongodb-mms-atlas-restore.service
%attr(0644,root,root) /etc/logrotate.d/mongodb-mms-atlas-restore
%attr(0600,atlas-restore,atlas-restore) %config(noreplace) %verify(not md5 size mtime) /etc/mongodb-mms/atlas-restore.yaml
%dir %attr(0755,atlas-restore,atlas-restore) /var/log/mongodb-mms-atlas-restore
%dir %attr(0755,atlas-restore,atlas-restore) /var/lib/mongodb-mms-atlas-restore

%post
# On install
if test $1 = 1; then
    setcap cap_net_bind_service+ep /opt/mongodb-mms-atlas-restore/bin/mongodb-mms-atlas-restore
    systemctl daemon-reload >/dev/null 2>&1
    systemctl preset mongodb-mms-atlas-restore.service >/dev/null 2>&1
fi
exit 0

%preun
# On uninstall stop the service
if test $1 = 0; then
    systemctl --no-reload disable mongodb-mms-atlas-restore.service > /dev/null 2>&1
    systemctl stop mongodb-mms-atlas-restore.service > /dev/null 2>&1
fi
exit 0

%postun
systemctl daemon-reload >/dev/null 2>&1

if test $1 = 1; then
    systemctl try-restart mongodb-mms-atlas-restore >/dev/null 2>&1
fi
exit 0
