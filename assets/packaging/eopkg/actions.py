#!/usr/bin/python

from pisi.actionsapi import pisitools, shelltools


def setup():
    pass


def install():
    pisitools.insinto("/usr/sbin/", "spotitube")
