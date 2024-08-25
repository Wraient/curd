import os
import json
import socket
import subprocess

def start_video(link, salt):
    # print(f"SALT IS {salt}")
    subprocess.Popen(['alacritty', '-e', 'bash', '-c', f"mpv --input-ipc-server=/tmp/mpvsocket{salt} {link}"])
    # os.system()
    # Path to the MPV socket
    # socket_path = f'/tmp/mpvsocket{salt}'
    # Connect to the MPV IPC socket
    # client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    # client.connect(socket_path)
