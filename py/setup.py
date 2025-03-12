#!/usr/bin/env python
# Copyright 2023 The Overlord Authors. All rights reserved.

from setuptools import setup
import subprocess
import sys

# Check if we're installing
if 'install' in sys.argv or 'develop' in sys.argv:
    # Try to install ws4py from git directly
    print("Installing ws4py dependency from git...")
    try:
        subprocess.check_call([
            sys.executable, '-m', 'pip', 'install',
            'git+https://github.com/aitjcize/WebSocket-for-Python.git#egg=ws4py'
        ])
        print("Successfully installed ws4py")
    except subprocess.CalledProcessError as e:
        print(f"Warning: Failed to install ws4py dependency: {e}")
        print("You may need to install it manually with:")
        print("pip install git+https://github.com/aitjcize/WebSocket-for-Python.git")

# Read requirements from requirements.txt
with open('requirements.txt') as f:
    requirements = f.read().splitlines()

setup(
    name="overlord",
    version="1.0.0",
    description="Overlord - Remote Device Management and Control System",
    author="Wei-Ning Huang",
    author_email="aitjcize@gmail.com",
    url="https://github.com/aitjcize/Overlord",
    py_modules=["ovl", "ghost", "update_ui_status", "relay_overlord_discovery_packet", "stream_camera"],
    entry_points={
        'console_scripts': [
            'ovl=ovl:main',
            'ghost=ghost:main',
            'update_ui_status=update_ui_status:main',
            'relay_overlord_discovery_packet=relay_overlord_discovery_packet:main',
            'stream_camera=stream_camera:main',
        ],
    },
    install_requires=requirements,
    python_requires='>=3.6',
    classifiers=[
        "Development Status :: 4 - Beta",
        "Environment :: Console",
        "Intended Audience :: Developers",
        "License :: OSI Approved :: BSD License",
        "Operating System :: OS Independent",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.6",
        "Programming Language :: Python :: 3.7",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Topic :: Software Development :: Testing",
        "Topic :: System :: Monitoring",
        "Topic :: System :: Systems Administration",
    ],
    dependency_links=[
        "https://github.com/aitjcize/WebSocket-for-Python/tarball/master#egg=ws4py-0.5.1"
    ],
)
