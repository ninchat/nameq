PREFIX		:= /usr/local
BINDIR		:= $(PREFIX)/bin
DESTDIR		:=

build::
all::

install::
	install -m 755 nameq.py $(DESTDIR)$(BINDIR)/nameq

clean::
	rm -f *.py[co]
