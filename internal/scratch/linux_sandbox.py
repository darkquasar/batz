"""Apply an inherited, unprivileged Linux amd64 policy, then exec the child.

Landlock ABI 3 or newer is mandatory, including file truncation restrictions.
Seccomp also denies sockets, process-memory access and asynchronous I/O bypasses.
No fallback executes a child without both policies.
"""
import ctypes
import errno
import json
import os
import platform
import stat
import sys


def runtime_facts():
    # DuckDB 1.5.4 probes CPU/memory limits during import and connection setup.
    # Admit individual kernel fact files, never their containing directories.
    found = {'/proc/self/cgroup', '/sys/devices/system/cpu/online'}
    with open('/proc/self/cgroup') as handle:
        for line in handle:
            parts = line.strip().split(':', 2)
            if len(parts) != 3:
                raise RuntimeError('invalid kernel cgroup entry')
            hierarchy, controllers, path = parts
            path = '' if path == '/' else path
            names = controllers.split(',')
            if hierarchy == '0' and names == ['']:
                found.update('/sys/fs/cgroup%s/%s' % (p, name)
                             for p in (path, '') for name in ('memory.max', 'cpu.max'))
            if 'memory' in names:
                found.update('/sys/fs/cgroup/memory%s/memory.limit_in_bytes' % p for p in (path, ''))
            if 'cpu' in names:
                found.update('/sys/fs/cgroup/cpu%s/%s' % (p, name)
                             for p in (path, '') for name in ('cpu.cfs_quota_us', 'cpu.cfs_period_us'))
    result = set()
    for path in found:
        resolved = os.path.realpath(path)
        if path.startswith('/sys/fs/cgroup/') and not resolved.startswith('/sys/fs/cgroup/'):
            continue
        result.add(resolved)
    return sorted(result)


def confine(policy):
    if sys.platform != 'linux' or platform.machine() != 'x86_64':
        raise RuntimeError('Linux sandbox requires native amd64')
    libc = ctypes.CDLL(None, use_errno=True)
    libc.syscall.restype = ctypes.c_long

    def checked(result, operation):
        if result < 0:
            code = ctypes.get_errno()
            raise OSError(code, operation + ': ' + os.strerror(code))
        return result

    abi = checked(libc.syscall(444, 0, 0, 1), 'Landlock ABI query')
    if abi < 3:
        raise RuntimeError('Linux sandbox requires Landlock ABI >= 3')
    # ABI 3 handles every filesystem right through TRUNCATE (bit 14).
    filesystem = (1 << 15) - 1
    readonly = (1 << 0) | (1 << 2) | (1 << 3)

    class Ruleset(ctypes.Structure):
        _fields_ = [('handled_access_fs', ctypes.c_uint64)]

    class PathRule(ctypes.Structure):
        _pack_ = 1
        _fields_ = [('allowed_access', ctypes.c_uint64), ('parent_fd', ctypes.c_int)]

    attr = Ruleset(filesystem)
    ruleset = checked(libc.syscall(444, ctypes.byref(attr), ctypes.sizeof(attr), 0),
                      'Landlock create ruleset')
    try:
        def allow(path, rights, optional=False, file_only=False):
            try:
                fd = os.open(path, os.O_PATH | os.O_CLOEXEC)
            except FileNotFoundError:
                if optional:
                    return
                raise
            try:
                mode = os.fstat(fd).st_mode
                if file_only and not stat.S_ISREG(mode):
                    raise RuntimeError('runtime fact must be a regular file: ' + path)
                if not stat.S_ISDIR(mode):
                    rights &= (1 << 0) | (1 << 1) | (1 << 2) | (1 << 14)
                rule = PathRule(rights, fd)
                checked(libc.syscall(445, ruleset, 1, ctypes.byref(rule), 0),
                        'Landlock add path rule')
            finally:
                os.close(fd)

        for path in ('/usr/lib', '/usr/share', '/lib', '/lib64',
                     '/etc/ld.so.cache', '/etc/localtime', '/dev/urandom', '/dev/random'):
            allow(path, readonly, optional=True)
        for path in policy['reads']:
            allow(path, readonly)
        for path in runtime_facts():
            allow(path, 1 << 2, optional=True, file_only=True)
        allow('/dev/null', (1 << 1) | (1 << 2))
        allow(policy['root'], filesystem)
        checked(libc.prctl(38, 1, 0, 0, 0), 'no_new_privs')
        checked(libc.syscall(446, ruleset, 0), 'Landlock restrict self')
    finally:
        os.close(ruleset)

    class Filter(ctypes.Structure):
        _fields_ = [('code', ctypes.c_ushort), ('jt', ctypes.c_ubyte),
                    ('jf', ctypes.c_ubyte), ('k', ctypes.c_uint32)]

    class Program(ctypes.Structure):
        _fields_ = [('length', ctypes.c_ushort), ('filters', ctypes.POINTER(Filter))]

    # Check the syscall ABI before interpreting amd64 syscall numbers. Reject
    # x32 and other architectures instead of accidentally admitting their calls.
    instructions = [(0x20, 0, 0, 4), (0x15, 1, 0, 0xC000003E),
                    (0x06, 0, 0, 0x80000000), (0x20, 0, 0, 0),
                    (0x35, 0, 1, 0x40000000), (0x06, 0, 0, 0x80000000)]
    # CPython's script fopen uses FIOCLEX to set this process's descriptor
    # close-on-exec bit. Admit only that descriptor operation; filesystem
    # metadata ioctls remain denied even on read-only admitted descriptors.
    instructions.extend([(0x15, 0, 4, 16), (0x20, 0, 0, 24),
                         (0x15, 1, 0, 0x5451),
                         (0x06, 0, 0, 0x50000 | errno.EPERM),
                         (0x06, 0, 0, 0x7FFF0000)])
    denied = [*range(41, 56), 101, 288, 299, 307, 310, 311, 438,
              425, 426, 427,
              # Landlock does not mediate metadata changes. Deny these globally.
              90, 91, 92, 93, 94, 132, 235, 260, 261, 268, 280, 452,
              188, 189, 190, 197, 198, 199]
    for number in denied:
        instructions.extend([(0x15, 0, 1, number), (0x06, 0, 0, 0x50000 | errno.EPERM)])
    instructions.append((0x06, 0, 0, 0x7FFF0000))
    filters = (Filter * len(instructions))(*(Filter(*row) for row in instructions))
    program = Program(len(filters), filters)
    checked(libc.prctl(22, 2, ctypes.byref(program), 0, 0), 'seccomp filter')


if __name__ == '__main__':
    confine(json.loads(sys.argv[1]))
    os.execv(sys.argv[2], sys.argv[2:])
