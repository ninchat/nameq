CXX		:= c++
AR		:= ar

CPPFLAGS	:= -DNDEBUG -DBOOST_DISABLE_ASSERTS
CFLAGS		:= -g -Wall -Wextra -O2 -fvisibility=hidden
LDFLAGS		:=
LIBS		:= -lboost_filesystem -lboost_system

build: nameq.a nameq.pic.a nameq.so

nameq.a: nameq.o
	$(AR) crs $@ nameq.o

nameq.pic.a: nameq.os
	$(AR) crs $@ nameq.os

nameq.so: nameq.os
	$(CXX) $(CFLAGS) $(LDFLAGS) -fPIC -shared -Wl,-soname,$@.0 -o $@ nameq.os $(LIBS)

nameq.o: nameq.cpp nameq.hpp
	$(CXX) $(CPPFLAGS) $(CFLAGS) -c -o $@ nameq.cpp

nameq.os: nameq.cpp nameq.hpp
	$(CXX) $(CPPFLAGS) -DPIC $(CFLAGS) -fPIC -c -o $@ nameq.cpp

test: nameq_test
	./nameq_test

nameq_test: nameq_test.cpp nameq.hpp nameq.a
	$(CXX) $(CPPFLAGS) $(CFLAGS) $(LDFLAGS) -o $@ nameq_test.cpp nameq.a $(LIBS)

clean:
	rm -f nameq.a nameq.pic.a nameq.so nameq.o nameq.os nameq_test

.PHONY: build test clean
