import os
import json
import socket

os.system("mpv --input-ipc-server=/tmp/mpvsocket 'https://video.wixstatic.com/video/53f59c_bddcaac9d0b940c8b0238a429a47177e/1080p/mp4/file.mp4'")
# Path to the MPV socket
socket_path = '/tmp/mpvsocket'

# Connect to the MPV IPC socket
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect(socket_path)

def send_command(command):
    """Send a command to MPV and return the response."""
    client.send((json.dumps(command) + '\n').encode('utf-8'))
    response = client.recv(1024)
    return json.loads(response.decode('utf-8'))

def get_time_pos():
    """Get the current playback time position."""
    command = {"command": ["get_property", "time-pos"]}
    response = send_command(command)
    return response.get('data', 0)

def pause():
    """Pause the playback."""
    command = {"command": ["set_property", "pause", True]}
    send_command(command)

def play():
    """Resume the playback."""
    command = {"command": ["set_property", "pause", False]}
    send_command(command)

def stop():
    """Stop the playback."""
    command = {"command": ["quit"]}
    send_command(command)

try:
    while True:
        time_pos = get_time_pos()
        print(f"Current Time: {time_pos:.2f} seconds")
        # Add any control commands here, for example, to pause or resume
        # pause() or play() or stop()
        time.sleep(1)
except KeyboardInterrupt:
    stop()
finally:
    client.close()
