.DEFAULT_GOAL := all
PROJECT_ROOT := src/

NAME := spotitube
GOARCH := amd64
VERSION := $(shell awk -F'= ' '/VERSION / {print $$2}' src/spotitube/constants.go)

BINARIES = \
	main

include Makefile.gobuild

_PKGS = \
	spotitube

_DEPENDENCIES = \
	github.com/zmb3/spotify \
	github.com/bogem/id3v2 \
	github.com/PuerkitoBio/goquery \
	github.com/fatih/color \
	github.com/kennygrant/sanitize \
	github.com/AlecAivazis/survey

_CHECK_DEPENDENCIES = $(addsuffix .dependency,$(_DEPENDENCIES))

_CHECK_COMPLIANCE = $(addsuffix .compliant,$(_PKGS))

BINS = $(addsuffix .statbin,$(BINARIES))

dependencies: $(_CHECK_DEPENDENCIES)

compliant: $(_CHECK_COMPLIANCE)

install: $(BINS)
	@ ( \
		test -d $(DESTDIR)/usr/local/bin || install -D -d -m 00755 $(DESTDIR)/usr/local/bin; \
		install -m 00755 bin/* $(DESTDIR)/usr/local/bin/; \
	);

x86: GOARCH=386
x86: all

x64: all

create_out:
	@ ( \
		mkdir -p out \
	);

package_rpm: create_out
	@ ( \
		cp package/rpm/spotitube.spec{,.orig}; \
		sed -i 's/:VERSION:/$(VERSION)/g' package/rpm/spotitube.spec; \
		make x86; \
		rpmbuild -ba --target=i386 package/rpm/spotitube.spec; \
		mv ~/rpmbuild/RPMS/i386/*.rpm out/spotitube-i386.rpm; \
		make x64; \
		rpmbuild -ba --target=x86_64 package/rpm/spotitube.spec; \
		mv ~/rpmbuild/RPMS/x86_64/*.rpm out/spotitube-x86_64.rpm; \
		rm -rf ~/rpmbuild; \
		rm -f package/rpm/spotitube.spec; \
		mv package/rpm/spotitube.spec{.orig,}; \
	);

package_deb: create_out
	@ ( \
		cp package/deb/DEBIAN/control{,.orig}; \
		sed -i 's/:VERSION:/$(VERSION)/g' package/deb/DEBIAN/control; \
		make x86; \
		cd package/deb; \
		mkdir -p usr/sbin; \
		cp ../../bin/spotitube usr/sbin/; \
		dpkg-deb --build . ../../out/spotitube-i386.deb; \
		rm -f usr/sbin/*; \
		cd ../..; \
		make x64; \
		cd package/deb; \
		cp ../../bin/spotitube usr/sbin/; \
		dpkg-deb --build . ../../out/spotitube-amd64.deb; \
		cd ../..; \
		rm -rf package/deb/usr; \
		rm -f package/deb/DEBIAN/control; \
		mv package/deb/DEBIAN/control{.orig,}; \
	);

package_eopkg: create_out
	@ ( \
		cp package/eopkg/pspec.xml{,.orig}; \
		sed -i 's/:VERSION:/$(VERSION)/g' package/eopkg/pspec.xml; \
		make x86; \
		cp bin/spotitube package/eopkg/files/; \
		sudo solbuild build package/eopkg/pspec.xml; \
		mv spotitube-*.eopkg out/spotitube-i386.eopkg; \
		make x64; \
		sudo solbuild build package/eopkg/pspec.xml; \
		mv spotitube-*.eopkg out/spotitube-x86_64.eopkg; \
		rm -f package/eopkg/pspec.xml; \
		mv package/eopkg/pspec.xml{.orig,}; \
		rm -f package/eopkg/files/spotitube; \
	);

package_snap: create_out

packages: create_out package_rpm package_deb package_eopkg package_snap

all: dependencies $(BINS)
