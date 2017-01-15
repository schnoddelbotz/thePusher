/*

  example configuration for thePusher master

  lists images available and client groups for deployment

  COPY THIS FILE TO: /path/to/your/images/thePusher-config.hcl
  The /path/to/your/images must be passed to master using -S / --image-storage flag

*/

### IMAGES ####################################################################

# provide a unique name per image to be deployed
image "ubuntu1604" {

  # filename of image inside folder specified using -S / --image-storage
  filename    = "ubuntu1604-sda1.img"

  # comment for personal reference and display on clients
  comment     = "Ubuntu 16.04 LTS"

  # destination device or folder for image
  destination = "/dev/sda1"

  # image types supported: IMG | TAR
  type        = "IMG"

  # compression may be one of NONE | BZ2 | GZIP
  compression = "NONE"

  # preImage and postImage will be executed using sh -c "...commands..."
  # use -s / --static-content flag to master to let master share scripts etc. via /static.
  # Occurrences of #MASTER# will be replaced by master's IP address.
  preImage    = "wget http://#MASTER#:8080/static/preimage-script-1.sh; sh preimage-script1.sh"
  postImage   = "sleep 5; eject ; reboot"
}

# another example image
image "fedora25" {
  filename    = "fedora25.img.bz2"
  comment     = "Fedora 25 2016-12-11"
  destination = "/tmp/dat2"
  type        = "IMG"
  compression = "BZ2"
}

### CLIENT GROUPS #############################################################

# provide a unique name per client group
clientgroup "test1" {

  # the referred image must be defined above
  image = "ubuntu1604"

  # list of hosts who will receive named image
  # ideally, the last client in list should be booted last (as it triggers image stream)
  hosts = ["192.168.78.158","192.168.78.133", "192.168.78.134"]
}

# another client group example
clientgroup "roomA" {
  image = "fedora25"
  hosts = ["192.168.78.140", "192.168.78.141", "192.168.78.142"]
}
