import os
import json
import socket
import random

def start_video(link):
    salt = random.randint(0,500)
    os.system(f"mpv --input-ipc-server=/tmp/mpvsocket{salt} {link}")
    # Path to the MPV socket
    socket_path = f'/tmp/mpvsocket{salt}'

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

# def get_data():

