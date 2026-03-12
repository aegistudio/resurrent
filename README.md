# Resurrent: Two-Stage Snapshot FileSystem

## Directory Structure

The directory structure when viewing from the root
of a resurrent filesystem looks like (ending with
`/` means it is a directory):

- `resurrent.yaml`: Stores the metadata of the
  whole filesystem. It should be under version control.
- `resurrent.ver`: Stores the current version
  of the filesystem structure. It should be under
  version control.
- `resurrent.lock`: Runtime lock that prohibits
  multiple active instances of the filesystem.
  It should be ignored by version control.
- `files/`: Stores the data and metadata of the
  files, including basic information, filesystem
  specific attributes and snapshots. It should be
  under version control.
- `objs/`: Stores the snapshotted objects that are
  indexed, including the secure hashing information
  and object storage information. It should be
  under version control.
- `temp/`: Temporary data generated during
  the filesystem execution. It is cleared upon
  shutdown of startup of the filesystem. It is
  transient and should be ignored by
  version control.

Every files and directories in `files/` must
have proper prefix to help avoid name collisions:

- The file storing the file content must have
  prefix `F.`.
- The file storing the metadata must have
  prefix `M.`.
- The directory must have prefix `D.`.

There can be other files under the root directory,
such as `.git` and `.gitignore` when `git` is used
for version control.

## Snapshotting and Recovering

The most important feature of the filesystem is
to create snapshots and recovering them. The
files, stored under `local/data/` and `local/meta/`,
may vary as they like, until we create a
snapshot out of them.

For every file being snapshotted, the workflow
will be like:

- Some secure hashing operations (e.g., SHA-256)
  will be performed, and then the `objs/` will
  be looked up and matched for existing
  snapshotted objects. If a matching snapshotted
  object is found, it will be reused.
- If a matching snapshotted object is not found,
  the file will be staged for object storage, based
  on the setting of `resurrent.yaml`. Once the
  file content has been stored in at least one
  object storage, a new snapshotted object will
  be created under `objs/` directory, plus the
  information of where it is stored.
- The metadata under `meta/` will be modified,
  the ID of snapshotted object will be appended
  to the metadata, indicating a new version of
  the file has been created.

Specially, if the file is too small (based on
the `inline_max_size` setting of `resurrent.yaml`,
which defaults to `32KB` after DEFLATE),
then it will be inlined in the file directly.