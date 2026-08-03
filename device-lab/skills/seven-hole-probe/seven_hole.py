# -*- coding: utf-8 -*-
"""
七孔探针校准算法 —— 原始 Python 参考实现。

来源：原项目 docs/seven_hole.py，迁入 device-lab/skills/seven-hole-probe/ 作为
SKILL.md 算法说明的可执行参考实现。SKILL.md 描述算法逻辑，本文件提供与之一一
对应的 Python 代码，便于校验 Go 实现的数值正确性。

注意：本文件依赖 sympy（见下方 import），Kimi Work 受管 Python 运行时未预装；
此处作为只读参考实现保存，供公式对照与数值校验，不要求直接运行。

文件中保留了历史 GPT3.5 注释版（带 """ """ 包裹）与健壮版（try-except）两套
实现，二者算法逻辑等价，健壮版仅增加异常处理与参数校验。如需对照公式，请优先
阅读未注释的健壮版函数体。
"""

import math
import os
from sympy import symbols, Eq, solve
import json

"""
def cal_ab(hole_dict_path, calibration_path):
    hole_dict = json_to_dict(read_txt_file(hole_dict_path))

    little_calibration_list = little_read_file(calibration_path)

    little_point_dict = little_cal_kakb(hole_dict)

    little_line_list = little_create_line(little_calibration_list)

    little_sign = point_in_polygon(little_point_dict, little_line_list)

    if little_sign == 0 or little_sign == 1:

        little_square_list = little_create_square(little_calibration_list)

        little_point_dict = little_cal_ab(little_point_dict, little_square_list)

        little_square_dict = little_cptcps_square(little_point_dict, little_calibration_list)

        little_point_dict = little_cal_cptcps(little_point_dict, little_square_dict)

        little_point_dict = little_cal_ptps(little_point_dict, hole_dict)

        little_point_dict = cal_velocity_mach(hole_dict, little_point_dict)

        little_point_dict = dict_to_json(little_point_dict)

        return little_point_dict

    else:

        big_calibration_dict = big_read_file(calibration_path)

        max_keys = big_max_pressure(hole_dict)

        big_first_point_dict = big_cal_kakb(hole_dict, max_keys['first'])

        big_first_line_list = big_create_line(big_calibration_dict, max_keys['first'])

        big_first_sign = point_in_polygon(big_first_point_dict, big_first_line_list)

        if big_first_sign == 0 or big_first_sign == 1:

            big_first_square_list = big_create_square(big_calibration_dict, max_keys['first'])

            big_first_point_dict = big_cal_ab(big_first_point_dict, big_first_square_list)

            big_first_square_dict = big_cptcps_square(big_first_point_dict, big_calibration_dict, max_keys['first'])

            big_first_point_dict = big_cal_cptcps(big_first_point_dict, big_first_square_dict)

            big_first_point_dict = big_cal_ptps(big_first_point_dict, hole_dict, max_keys['first'])

            big_first_point_dict = big_ab_convert(big_first_point_dict)

            big_first_point_dict = cal_velocity_mach(hole_dict, big_first_point_dict)

            big_first_point_dict = dict_to_json(big_first_point_dict)

            return big_first_point_dict

        else:

            big_second_point_dict = big_cal_kakb(hole_dict, max_keys['second'])

            big_second_line_list = big_create_line(big_calibration_dict, max_keys['second'])

            big_second_sign = point_in_polygon(big_second_point_dict, big_second_line_list)

            if big_second_sign == 0 or big_second_sign == 1:

                big_second_square_list = big_create_square(big_calibration_dict, max_keys['second'])

                big_second_point_dict = big_cal_ab(big_second_point_dict, big_second_square_list)

                big_second_square_dict = big_cptcps_square(big_second_point_dict, big_calibration_dict,
                                                           max_keys['second'])

                big_second_point_dict = big_cal_cptcps(big_second_point_dict, big_second_square_dict)

                big_second_point_dict = big_cal_ptps(big_second_point_dict, hole_dict, max_keys['second'])

                big_second_point_dict = big_ab_convert(big_second_point_dict)

                big_second_point_dict = cal_velocity_mach(hole_dict, big_second_point_dict)

                big_second_point_dict = dict_to_json(big_second_point_dict)

                return big_second_point_dict

            else:

                big_first_point_dict = beyond_border(big_first_point_dict, big_first_line_list, max_keys['first'],
                                                     big_calibration_dict, hole_dict)

                return big_first_point_dict

    return "no-return"
"""
def cal_ab(hole_dict_path, calibration_path):
    try:
        hole_dict = json_to_dict(read_txt_file(hole_dict_path))
        little_calibration_list = little_read_file(calibration_path)
        little_point_dict = little_cal_kakb(hole_dict)
        little_line_list = little_create_line(little_calibration_list)
        little_sign = point_in_polygon(little_point_dict, little_line_list)

        if little_sign == 0 or little_sign == 1:
            little_square_list = little_create_square(little_calibration_list)
            little_point_dict = little_cal_ab(little_point_dict, little_square_list)
            little_square_dict = little_cptcps_square(little_point_dict, little_calibration_list)
            little_point_dict = little_cal_cptcps(little_point_dict, little_square_dict)
            little_point_dict = little_cal_ptps(little_point_dict, hole_dict)
            little_point_dict = cal_velocity_mach(hole_dict, little_point_dict)
            little_point_dict = dict_to_json(little_point_dict)
            return little_point_dict
        else:
            big_calibration_dict = big_read_file(calibration_path)
            max_keys = big_max_pressure(hole_dict)
            big_first_point_dict = big_cal_kakb(hole_dict, max_keys['first'])
            big_first_line_list = big_create_line(big_calibration_dict, max_keys['first'])
            big_first_sign = point_in_polygon(big_first_point_dict, big_first_line_list)

            if big_first_sign == 0 or big_first_sign == 1:
                big_first_square_list = big_create_square(big_calibration_dict, max_keys['first'])
                big_first_point_dict = big_cal_ab(big_first_point_dict, big_first_square_list)
                big_first_square_dict = big_cptcps_square(big_first_point_dict, big_calibration_dict, max_keys['first'])
                big_first_point_dict = big_cal_cptcps(big_first_point_dict, big_first_square_dict)
                big_first_point_dict = big_cal_ptps(big_first_point_dict, hole_dict, max_keys['first'])
                big_first_point_dict = big_ab_convert(big_first_point_dict)
                big_first_point_dict = cal_velocity_mach(hole_dict, big_first_point_dict)
                big_first_point_dict = dict_to_json(big_first_point_dict)
                return big_first_point_dict
            else:
                big_second_point_dict = big_cal_kakb(hole_dict, max_keys['second'])
                big_second_line_list = big_create_line(big_calibration_dict, max_keys['second'])
                big_second_sign = point_in_polygon(big_second_point_dict, big_second_line_list)

                if big_second_sign == 0 or big_second_sign == 1:
                    big_second_square_list = big_create_square(big_calibration_dict, max_keys['second'])
                    big_second_point_dict = big_cal_ab(big_second_point_dict, big_second_square_list)
                    big_second_square_dict = big_cptcps_square(big_second_point_dict, big_calibration_dict, max_keys['second'])
                    big_second_point_dict = big_cal_cptcps(big_second_point_dict, big_second_square_dict)
                    big_second_point_dict = big_cal_ptps(big_second_point_dict, hole_dict, max_keys['second'])
                    big_second_point_dict = big_ab_convert(big_second_point_dict)
                    big_second_point_dict = cal_velocity_mach(hole_dict, big_second_point_dict)
                    big_second_point_dict = dict_to_json(big_second_point_dict)
                    return big_second_point_dict
                else:
                    big_first_point_dict = beyond_border(big_first_point_dict, big_first_line_list, max_keys['first'],
                                                         big_calibration_dict, hole_dict)
                    return big_first_point_dict

        return "no-return"
    except KeyError as e:
        print(f"KeyError: {e}")
    except ValueError as e:
        print(f"ValueError: {e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")
        return "no-return"

"""
GPT3.5
def beyond_border(big_first_point_dict, big_first_line_list, n, big_calibration_dict, hole_dict):
    fai_list = [0, 60, 120, 180, 240, 300]
    ka = float(big_first_point_dict['ka'])
    kb = float(big_first_point_dict['kb'])
    big_first_point_dict['a'] = 45.0
    kb_right_list = big_first_line_list[1]
    kb_min = float(kb_right_list[0]['kb'])
    kb_max = float(kb_right_list[12]['kb'])
    if kb > kb_max or kb < kb_min:
        big_first_point_dict['b'] = fai_list[int(n) - 1]
    else:
        for i in range(len(big_first_line_list[1])):
            if kb >= float(big_first_line_list[1][i]['kb']) and kb <= float(big_first_line_list[1][i + 1]['kb']):
                big_first_point_dict['b'] = -5 * (kb - float(big_first_line_list[1][i]['kb'])) / (
                            float(big_first_line_list[1][i + 1]['kb']) - float(
                        big_first_line_list[1][i]['kb'])) + float(big_first_line_list[1][i]['b'])
                break
    big_first_square_dict = big_cptcps_square(big_first_point_dict, big_calibration_dict, n)

    big_first_point_dict = big_cal_cptcps(big_first_point_dict, big_first_square_dict)

    big_first_point_dict = big_cal_ptps(big_first_point_dict, hole_dict, n)

    big_first_point_dict = big_ab_convert(big_first_point_dict)

    big_first_point_dict = cal_velocity_mach(hole_dict, big_first_point_dict)

    big_first_point_dict = dict_to_json(big_first_point_dict)

    return big_first_point_dict
"""
def beyond_border(big_first_point_dict, big_first_line_list, n, big_calibration_dict, hole_dict):
    try:
        fai_list = [0, 60, 120, 180, 240, 300]
        ka = float(big_first_point_dict['ka'])
        kb = float(big_first_point_dict['kb'])
        
        # Assume the initial value of 'a'
        big_first_point_dict['a'] = 45.0
        
        kb_right_list = big_first_line_list[1]
        kb_min = float(kb_right_list[0]['kb'])
        kb_max = float(kb_right_list[12]['kb'])
        
        # Check if 'kb' is out of bounds
        if kb > kb_max or kb < kb_min:
            big_first_point_dict['b'] = fai_list[int(n) - 1]
        else:
            for i in range(len(kb_right_list)):
                if kb >= float(kb_right_list[i]['kb']) and kb <= float(kb_right_list[i + 1]['kb']):
                    big_first_point_dict['b'] = -5 * (kb - float(kb_right_list[i]['kb'])) / (
                                float(kb_right_list[i + 1]['kb']) - float(kb_right_list[i]['kb'])) + float(
                        kb_right_list[i]['b'])
                    break
        
        # Calculate big_first_square_dict
        big_first_square_dict = big_cptcps_square(big_first_point_dict, big_calibration_dict, n)
        
        # Calculate big_first_point_dict['cpt'] and big_first_point_dict['cps']
        big_first_point_dict = big_cal_cptcps(big_first_point_dict, big_first_square_dict)
        
        # Calculate big_first_point_dict['pt'] and big_first_point_dict['ps']
        big_first_point_dict = big_cal_ptps(big_first_point_dict, hole_dict, n)
        
        # Convert 'a' and 'b' to 'theta' and 'fai'
        big_first_point_dict = big_ab_convert(big_first_point_dict)
        
        # Calculate velocity and Mach number
        big_first_point_dict = cal_velocity_mach(hole_dict, big_first_point_dict)
        
        # Convert dict to JSON format
        big_first_point_dict = dict_to_json(big_first_point_dict)

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except IndexError as e:
        print(f"IndexError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None

    return big_first_point_dict


def read_txt_file(hole_dict_path):
    try:
        with open(hole_dict_path, 'r') as file:
            file_content = file.read()
        return file_content
    except FileNotFoundError:
        print("Error: File not found.")
        return None
    except IOError:
        print("Error: Unable to read file.")
        return None


def json_to_dict(json_str):
    try:
        data_dict = json.loads(json_str)
        return data_dict
    except ValueError as e:
        print("Error: Invalid JSON string.")
        return None


def dict_to_json(data_dict):
    try:
        json_str = json.dumps(data_dict)
        return json_str
    except TypeError as e:
        print("Error: Invalid dictionary.")
        return None

"""
def cal_velocity_mach(hole_dict, point_dict):
    v = pow(2 * math.fabs(point_dict['pt'] - point_dict['ps']) * 287.06 * (hole_dict['t'] + 273.15) / hole_dict['pa'],
            0.5)
    ma = pow(
        5 * math.fabs(pow((point_dict['pt'] + hole_dict['pa']) / (point_dict['ps'] + hole_dict['pa']), 0.4 / 1.4) - 1),
        0.5)
    point_dict['v'] = v
    point_dict['ma'] = ma

    return point_dict
"""

def cal_velocity_mach(hole_dict, point_dict):
    try:
        # 检查字典中是否包含所需的键
        required_keys_hole = ['t', 'pa']
        required_keys_point = ['pt', 'ps']
        
        for key in required_keys_hole:
            if key not in hole_dict:
                raise KeyError(f"缺少hole_dict中的关键键: {key}")
        
        for key in required_keys_point:
            if key not in point_dict:
                raise KeyError(f"缺少point_dict中的关键键: {key}")

        # 计算速度v
        v = pow(2 * math.fabs(point_dict['pt'] - point_dict['ps']) * 287.06 * (hole_dict['t'] + 273.15) / hole_dict['pa'], 0.5)
        
        # 计算马赫数ma
        ma = pow(5 * math.fabs(pow((point_dict['pt'] + hole_dict['pa']) / (point_dict['ps'] + hole_dict['pa']), 0.4 / 1.4) - 1), 0.5)
        
        # 更新point_dict
        point_dict['v'] = v
        point_dict['ma'] = ma

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ZeroDivisionError as e:
        print("ZeroDivisionError: 除数不能为零")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None

    return point_dict

"""
GPT3.5
def big_cal_ptps(big_point_dict, hole_dict, n):
    cpt = big_point_dict['cpt']
    cps = big_point_dict['cps']
    p1 = hole_dict['p1']
    p2 = hole_dict['p2']
    p3 = hole_dict['p3']
    p4 = hole_dict['p4']
    p5 = hole_dict['p5']
    p6 = hole_dict['p6']
    p7 = hole_dict['p7']
    p_average = (p1 + p2 + p3 + p4 + p5 + p6) / 6

    pcenter = hole_dict['p' + str(n)]
    if int(n) == 1:
        pleft = p6
    else:
        pleft = hole_dict['p' + str(int(n) - 1)]
    if n == 6:
        pright = p1
    else:
        pright = hole_dict['p' + str(int(n) + 1)]

    pt, ps = symbols('pt ps')
    eq1 = Eq(cpt, (pcenter - pt) / (pt - ps))
    eq2 = Eq(cps, (ps - (pleft + pright) / 2) / (pt - ps))
    solution = solve((eq1, eq2), (pt, ps))
    pt_solution = solution[pt]
    ps_solution = solution[ps]
    big_point_dict['pt'] = float(pt_solution)
    big_point_dict['ps'] = float(ps_solution)
    return big_point_dict
"""
def big_cal_ptps(big_point_dict, hole_dict, n):
    try:
        cpt = big_point_dict['cpt']
        cps = big_point_dict['cps']
        p1 = hole_dict['p1']
        p2 = hole_dict['p2']
        p3 = hole_dict['p3']
        p4 = hole_dict['p4']
        p5 = hole_dict['p5']
        p6 = hole_dict['p6']
        p7 = hole_dict['p7']
        p_average = (p1 + p2 + p3 + p4 + p5 + p6) / 6

        pcenter = hole_dict['p' + str(n)]
        if str(n) == '1':
            pleft = p6
        else:
            pleft = hole_dict['p' + str(int(n) - 1)]
        if str(n) == '6':
            pright = p1
        else:
            pright = hole_dict['p' + str(int(n) + 1)]

        pt, ps = symbols('pt ps')
        eq1 = Eq(cpt, (pcenter - pt) / (pt - ps))
        eq2 = Eq(cps, (ps - (pleft + pright) / 2) / (pt - ps))
        solution = solve((eq1, eq2), (pt, ps))
        pt_solution = solution[pt]
        ps_solution = solution[ps]
        big_point_dict['pt'] = float(pt_solution)
        big_point_dict['ps'] = float(ps_solution)
        return big_point_dict
    
    except KeyError as e:
        print(f"KeyError: {e} not found in dictionary.")
    except ValueError as e:
        print(f"ValueError: {e}. Check if numeric values are correctly formatted.")
    except Exception as e:
        print(f"Unexpected error: {e}")
        raise  # Re-raise exception for debugging or logging purposes

"""
GPT3.5
def big_cptcps_square(big_point_dict, big_calibration_dict, n):
    a = big_point_dict['a']
    b = big_point_dict['b']

    big_square_dict = {'X1': {}, 'X2': {}, 'X3': {}, 'X4': {}}

    k = int(a / 5)
    if k != 9:
        big_square_dict['X1']['a'] = 5 * k
        big_square_dict['X2']['a'] = 5 * k
        big_square_dict['X3']['a'] = 5 * (k + 1)
        big_square_dict['X4']['a'] = 5 * (k + 1)
    else:
        big_square_dict['X1']['a'] = 5 * (k - 1)
        big_square_dict['X2']['a'] = 5 * (k - 1)
        big_square_dict['X3']['a'] = 5 * k
        big_square_dict['X4']['a'] = 5 * k

    l = int(b / 5)
    if l == 71:
        big_square_dict['X1']['b'] = 355
        big_square_dict['X2']['b'] = 0
        big_square_dict['X3']['b'] = 0
        big_square_dict['X4']['b'] = 355
    else:
        big_square_dict['X1']['b'] = 5 * l
        big_square_dict['X2']['b'] = 5 * (l + 1)
        big_square_dict['X3']['b'] = 5 * (l + 1)
        big_square_dict['X4']['b'] = 5 * l

    k = 0
    for calibration in big_calibration_dict[n]:
        if big_square_dict['X1']['b'] == float(calibration['b']) and big_square_dict['X1']['a'] == float(
                calibration['a']):
            big_square_dict['X1']['cpt'] = float(calibration['cpt'])
            big_square_dict['X1']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if big_square_dict['X2']['b'] == float(calibration['b']) and big_square_dict['X2']['a'] == float(
                calibration['a']):
            big_square_dict['X2']['cpt'] = float(calibration['cpt'])
            big_square_dict['X2']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if big_square_dict['X3']['b'] == float(calibration['b']) and big_square_dict['X3']['a'] == float(
                calibration['a']):
            big_square_dict['X3']['cpt'] = float(calibration['cpt'])
            big_square_dict['X3']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if big_square_dict['X4']['b'] == float(calibration['b']) and big_square_dict['X4']['a'] == float(
                calibration['a']):
            big_square_dict['X4']['cpt'] = float(calibration['cpt'])
            big_square_dict['X4']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if k == 4:
            break

    return big_square_dict
"""
def big_cptcps_square(big_point_dict, big_calibration_dict, n):
    try:
        a = big_point_dict['a']
        b = big_point_dict['b']

        big_square_dict = {'X1': {}, 'X2': {}, 'X3': {}, 'X4': {}}

        k = int(a / 5)
        if k != 9:
            big_square_dict['X1']['a'] = 5 * k
            big_square_dict['X2']['a'] = 5 * k
            big_square_dict['X3']['a'] = 5 * (k + 1)
            big_square_dict['X4']['a'] = 5 * (k + 1)
        else:
            big_square_dict['X1']['a'] = 5 * (k - 1)
            big_square_dict['X2']['a'] = 5 * (k - 1)
            big_square_dict['X3']['a'] = 5 * k
            big_square_dict['X4']['a'] = 5 * k

        l = int(b / 5)
        if l == 71:
            big_square_dict['X1']['b'] = 355
            big_square_dict['X2']['b'] = 0
            big_square_dict['X3']['b'] = 0
            big_square_dict['X4']['b'] = 355
        else:
            big_square_dict['X1']['b'] = 5 * l
            big_square_dict['X2']['b'] = 5 * (l + 1)
            big_square_dict['X3']['b'] = 5 * (l + 1)
            big_square_dict['X4']['b'] = 5 * l

        k = 0
        for calibration in big_calibration_dict[n]:
            if big_square_dict['X1']['b'] == float(calibration['b']) and big_square_dict['X1']['a'] == float(calibration['a']):
                big_square_dict['X1']['cpt'] = float(calibration['cpt'])
                big_square_dict['X1']['cps'] = float(calibration['cps'])
                k += 1
                continue
            if big_square_dict['X2']['b'] == float(calibration['b']) and big_square_dict['X2']['a'] == float(calibration['a']):
                big_square_dict['X2']['cpt'] = float(calibration['cpt'])
                big_square_dict['X2']['cps'] = float(calibration['cps'])
                k += 1
                continue
            if big_square_dict['X3']['b'] == float(calibration['b']) and big_square_dict['X3']['a'] == float(calibration['a']):
                big_square_dict['X3']['cpt'] = float(calibration['cpt'])
                big_square_dict['X3']['cps'] = float(calibration['cps'])
                k += 1
                continue
            if big_square_dict['X4']['b'] == float(calibration['b']) and big_square_dict['X4']['a'] == float(calibration['a']):
                big_square_dict['X4']['cpt'] = float(calibration['cpt'])
                big_square_dict['X4']['cps'] = float(calibration['cps'])
                k += 1
                continue
            if k == 4:
                break

        return big_square_dict
    
    except KeyError as e:
        print(f"KeyError: {e} not found in dictionary.")
    except ValueError as e:
        print(f"ValueError: {e}. Check if numeric values are correctly formatted.")
    except Exception as e:
        print(f"Unexpected error: {e}")
        raise  # Re-raise exception for debugging or logging purposes


"""
def little_cal_ptps(little_point_dict, hole_dict):
    cpt = little_point_dict['cpt']
    cps = little_point_dict['cps']
    p1 = hole_dict['p1']
    p2 = hole_dict['p2']
    p3 = hole_dict['p3']
    p4 = hole_dict['p4']
    p5 = hole_dict['p5']
    p6 = hole_dict['p6']
    p7 = hole_dict['p7']
    p_average = (p1 + p2 + p3 + p4 + p5 + p6) / 6
    pt, ps = symbols('pt ps')
    eq1 = Eq(cpt, (p7 - pt) / (pt - ps))
    eq2 = Eq(cps, (ps - p_average) / (pt - ps))
    solution = solve((eq1, eq2), (pt, ps))
    pt_solution = solution[pt]
    ps_solution = solution[ps]
    little_point_dict['pt'] = float(pt_solution)
    little_point_dict['ps'] = float(ps_solution)

    return little_point_dict
"""

def little_cal_ptps(little_point_dict, hole_dict):
    try:
        cpt = little_point_dict['cpt']
        cps = little_point_dict['cps']
        p1 = hole_dict['p1']
        p2 = hole_dict['p2']
        p3 = hole_dict['p3']
        p4 = hole_dict['p4']
        p5 = hole_dict['p5']
        p6 = hole_dict['p6']
        p7 = hole_dict['p7']
        p_average = (p1 + p2 + p3 + p4 + p5 + p6) / 6

        pt, ps = symbols('pt ps')
        eq1 = Eq(cpt, (p7 - pt) / (pt - ps))
        eq2 = Eq(cps, (ps - p_average) / (pt - ps))
        
        # Solve the equations
        solution = solve((eq1, eq2), (pt, ps))
        
        # Check if solution is found
        if solution:
            pt_solution = solution[pt]
            ps_solution = solution[ps]
            little_point_dict['pt'] = float(pt_solution)
            little_point_dict['ps'] = float(ps_solution)
        else:
            print("Equations have no real solutions.")
    
    except KeyError as e:
        print(f"KeyError: {e} not found in dictionary.")
    except ValueError as e:
        print(f"ValueError: {e}. Check if numeric values are correctly formatted.")
    except ZeroDivisionError as e:
        print(f"ZeroDivisionError: {e}. Check for division by zero in equations.")
    except Exception as e:
        print(f"Unexpected error: {e}")

    return little_point_dict

"""
GPT3.5

def little_cal_cptcps(little_point_dict, little_square_dict):
    cptcps_list = ['cpt', 'cps']
    for cptcps in cptcps_list:
        k1 = (float(little_square_dict['X2'][cptcps]) - float(little_square_dict['X1'][cptcps])) / (
                float(little_square_dict['X2']['a']) - float(little_square_dict['X1']['a']))
        b1 = little_square_dict['X1'][cptcps] - k1 * little_square_dict['X1']['a']
        k3 = (float(little_square_dict['X4'][cptcps]) - float(little_square_dict['X3'][cptcps])) / (
                float(little_square_dict['X4']['a']) - float(little_square_dict['X3']['a']))
        b3 = float(little_square_dict['X3'][cptcps]) - k3 * float(little_square_dict['X3']['a'])

        cp1 = k1 * little_point_dict['a'] + b1
        cp2 = k3 * little_point_dict['a'] + b3
        cp = (cp2 - cp1) * (little_point_dict['b'] - little_square_dict['X1']['b']) / 5 + cp1
        little_point_dict[cptcps] = cp

    return little_point_dict
"""
def little_cal_cptcps(little_point_dict, little_square_dict):
    try:
        cptcps_list = ['cpt', 'cps']
        for cptcps in cptcps_list:
            k1 = (float(little_square_dict['X2'][cptcps]) - float(little_square_dict['X1'][cptcps])) / (
                    float(little_square_dict['X2']['a']) - float(little_square_dict['X1']['a']))
            b1 = float(little_square_dict['X1'][cptcps]) - k1 * float(little_square_dict['X1']['a'])
            k3 = (float(little_square_dict['X4'][cptcps]) - float(little_square_dict['X3'][cptcps])) / (
                    float(little_square_dict['X4']['a']) - float(little_square_dict['X3']['a']))
            b3 = float(little_square_dict['X3'][cptcps]) - k3 * float(little_square_dict['X3']['a'])

            cp1 = k1 * little_point_dict['a'] + b1
            cp2 = k3 * little_point_dict['a'] + b3
            cp = (cp2 - cp1) * (little_point_dict['b'] - float(little_square_dict['X1']['b'])) / 5 + cp1
            little_point_dict[cptcps] = cp

    except KeyError as e:
        print(f"KeyError: {e} not found in little_square_dict. Check the keys in your dictionary.")
    except ValueError as e:
        print(f"ValueError: {e}. Check if numeric values are correctly formatted.")
    except ZeroDivisionError as e:
        print(f"ZeroDivisionError: {e}. Check for division by zero in slope calculations.")
    except Exception as e:
        print(f"Unexpected error: {e}")

    return little_point_dict



def little_cptcps_square(little_point_dict, little_calibration_list):
    a = little_point_dict['a']
    b = little_point_dict['b']
    little_square_dict = {'X1': {}, 'X2': {}, 'X3': {}, 'X4': {}}
    k = int(a / 5)
    if 0 <= a < 30 or a <= -30:
        little_square_dict['X1']['a'] = 5 * k
        little_square_dict['X2']['a'] = 5 * (k + 1)
        little_square_dict['X3']['a'] = 5 * (k + 1)
        little_square_dict['X4']['a'] = 5 * k
    else:
        little_square_dict['X1']['a'] = 5 * (k - 1)
        little_square_dict['X2']['a'] = 5 * k
        little_square_dict['X3']['a'] = 5 * k
        little_square_dict['X4']['a'] = 5 * (k - 1)

    l = int(b / 5)
    if 0 <= b < 30 or b <= -30:
        little_square_dict['X1']['b'] = 5 * l
        little_square_dict['X2']['b'] = 5 * l
        little_square_dict['X3']['b'] = 5 * (l + 1)
        little_square_dict['X4']['b'] = 5 * (l + 1)
    else:
        little_square_dict['X1']['b'] = 5 * (l - 1)
        little_square_dict['X2']['b'] = 5 * (l - 1)
        little_square_dict['X3']['b'] = 5 * l
        little_square_dict['X4']['b'] = 5 * l

    k = 0
    for calibration in little_calibration_list:
        if little_square_dict['X1']['b'] == float(calibration['b']) and little_square_dict['X1']['a'] == float(
                calibration['a']):
            little_square_dict['X1']['cpt'] = float(calibration['cpt'])
            little_square_dict['X1']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if little_square_dict['X2']['b'] == float(calibration['b']) and little_square_dict['X2']['a'] == float(
                calibration['a']):
            little_square_dict['X2']['cpt'] = float(calibration['cpt'])
            little_square_dict['X2']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if little_square_dict['X3']['b'] == float(calibration['b']) and little_square_dict['X3']['a'] == float(
                calibration['a']):
            little_square_dict['X3']['cpt'] = float(calibration['cpt'])
            little_square_dict['X3']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if little_square_dict['X4']['b'] == float(calibration['b']) and little_square_dict['X4']['a'] == float(
                calibration['a']):
            little_square_dict['X4']['cpt'] = float(calibration['cpt'])
            little_square_dict['X4']['cps'] = float(calibration['cps'])
            k += 1
            continue
        if k == 4:
            break

    return little_square_dict


def big_ab_convert(big_point_dict):
    theta = big_point_dict['a']
    fai = big_point_dict['b']
    big_point_dict['theta'] = theta
    big_point_dict['fai'] = fai
    theta_radians = math.radians(theta)
    fai_radians = math.radians(fai)
    tan_a = math.tan(theta_radians) * math.sin(fai_radians)
    tan_b = math.tan(theta_radians) * math.cos(fai_radians)
    a_radians = math.atan(tan_a)
    a = math.degrees(a_radians)
    b_radians = math.atan(tan_b)
    b = math.degrees(b_radians)
    # big_point_dict['a'] = a
    big_point_dict['a'] = -a
    big_point_dict['b'] = b

    return big_point_dict

"""
GPT3.5
def little_read_file(calibration_path):
    little_calibration_list = []
    for filename in os.listdir(calibration_path):
        filepath = os.path.join(calibration_path, filename)
        if os.path.isfile(filepath) and filename.endswith('.prb') and filename == '7.prb':
            with open(filepath, 'r') as file:
                next(file)
                for line in file:
                    data = line.strip().split()
                    my_dict = {'ka': data[0], 'kb': data[1], 'cpt': data[2], 'cps': data[3], 'a': data[4], 'b': data[5]}
                    little_calibration_list.append(my_dict)
    return little_calibration_list
"""


def little_read_file(calibration_path):
    little_calibration_list = []
    
    try:
        if not os.path.exists(calibration_path) or not os.path.isdir(calibration_path):
            raise FileNotFoundError(f"Directory '{calibration_path}' does not exist or is not a directory.")
        
        for filename in os.listdir(calibration_path):
            filepath = os.path.join(calibration_path, filename)
            if os.path.isfile(filepath) and filename.endswith('.prb') and filename == '7.prb':
                with open(filepath, 'r') as file:
                    next(file)  # skip the header line
                    for line in file:
                        data = line.strip().split()
                        if len(data) == 6:
                            my_dict = {
                                'ka': data[0],
                                'kb': data[1],
                                'cpt': data[2],
                                'cps': data[3],
                                'a': data[4],
                                'b': data[5]
                            }
                            little_calibration_list.append(my_dict)
                        else:
                            print(f"Ignoring invalid line in file '{filename}': {line.strip()}")
    
    except (FileNotFoundError, IOError) as e:
        print(f"Error reading files from '{calibration_path}': {e}")
    
    return little_calibration_list

"""
GPT3.5
def big_read_file(calibration_path):
    big_calibration_dict = {}
    for filename in os.listdir(calibration_path):
        big_calibration_list = []
        filepath = os.path.join(calibration_path, filename)
        if os.path.isfile(filepath) and filename.endswith('.prb') and filename != '7.prb':
            with open(filepath, 'r') as file:
                next(file)
                for line in file:
                    data = line.strip().split()
                    my_dict = {'ka': data[0], 'kb': data[1], 'cpt': data[2], 'cps': data[3], 'a': data[4], 'b': data[5]}
                    big_calibration_list.append(my_dict)
                big_calibration_dict[filename[:-4]] = big_calibration_list
    return big_calibration_dict
"""

def big_read_file(calibration_path):
    big_calibration_dict = {}
    
    try:
        if not os.path.exists(calibration_path) or not os.path.isdir(calibration_path):
            raise FileNotFoundError(f"Directory '{calibration_path}' does not exist or is not a directory.")
        
        for filename in os.listdir(calibration_path):
            big_calibration_list = []
            filepath = os.path.join(calibration_path, filename)
            if os.path.isfile(filepath) and filename.endswith('.prb') and filename != '7.prb':
                with open(filepath, 'r') as file:
                    next(file)  # skip the header line
                    for line in file:
                        data = line.strip().split()
                        if len(data) == 6:
                            my_dict = {
                                'ka': data[0],
                                'kb': data[1],
                                'cpt': data[2],
                                'cps': data[3],
                                'a': data[4],
                                'b': data[5]
                            }
                            big_calibration_list.append(my_dict)
                        else:
                            print(f"Ignoring invalid line in file '{filename}': {line.strip()}")

                    big_calibration_dict[filename[:-4]] = big_calibration_list
    
    except (FileNotFoundError, IOError) as e:
        print(f"Error reading files from '{calibration_path}': {e}")
    
    return big_calibration_dict


"""
GPT3.5 

def big_cal_ab(big_point_dict, big_square_list):
    for square in big_square_list:
        k1 = (float(square['X2']['kb']) - float(square['X1']['kb'])) / (
                float(square['X2']['ka']) - float(square['X1']['ka']))
        b1 = float(square['X1']['kb']) - k1 * float(square['X1']['ka'])
        k2 = (float(square['X3']['kb']) - float(square['X2']['kb'])) / (
                float(square['X3']['ka']) - float(square['X2']['ka']))
        b2 = float(square['X2']['kb']) - k2 * float(square['X2']['ka'])
        k3 = (float(square['X4']['kb']) - float(square['X3']['kb'])) / (
                float(square['X4']['ka']) - float(square['X3']['ka']))
        b3 = float(square['X3']['kb']) - k3 * float(square['X3']['ka'])
        k4 = (float(square['X1']['kb']) - float(square['X4']['kb'])) / (
                float(square['X1']['ka']) - float(square['X4']['ka']))
        b4 = float(square['X4']['kb']) - k4 * float(square['X4']['ka'])

        y1 = k1 * big_point_dict['ka'] + b1
        y3 = k3 * big_point_dict['ka'] + b3
        x2 = (big_point_dict['kb'] - b2) / k2
        x4 = (big_point_dict['kb'] - b4) / k4
        if y1 <= big_point_dict['kb'] <= y3 and x2 >= big_point_dict['ka'] >= x4:
            d1 = math.fabs(((-k1) * big_point_dict['ka'] + big_point_dict['kb'] - b1) / math.sqrt(k1 * k1 + 1))
            d2 = math.fabs(((-k2) * big_point_dict['ka'] + big_point_dict['kb'] - b2) / math.sqrt(k2 * k2 + 1))
            d3 = math.fabs(((-k3) * big_point_dict['ka'] + big_point_dict['kb'] - b3) / math.sqrt(k3 * k3 + 1))
            d4 = math.fabs(((-k4) * big_point_dict['ka'] + big_point_dict['kb'] - b4) / math.sqrt(k4 * k4 + 1))

            big_point_dict['b'] = square['X1']['b'] - d1 / (d1 + d3) * 5
            big_point_dict['a'] = square['X1']['a'] + d4 / (d2 + d4) * 5
            return big_point_dict
"""

def big_cal_ab(big_point_dict, big_square_list):
    try:
        for square in big_square_list:
            k1 = (float(square['X2']['kb']) - float(square['X1']['kb'])) / (
                    float(square['X2']['ka']) - float(square['X1']['ka']))
            b1 = float(square['X1']['kb']) - k1 * float(square['X1']['ka'])
            k2 = (float(square['X3']['kb']) - float(square['X2']['kb'])) / (
                    float(square['X3']['ka']) - float(square['X2']['ka']))
            b2 = float(square['X2']['kb']) - k2 * float(square['X2']['ka'])
            k3 = (float(square['X4']['kb']) - float(square['X3']['kb'])) / (
                    float(square['X4']['ka']) - float(square['X3']['ka']))
            b3 = float(square['X3']['kb']) - k3 * float(square['X3']['ka'])
            k4 = (float(square['X1']['kb']) - float(square['X4']['kb'])) / (
                    float(square['X1']['ka']) - float(square['X4']['ka']))
            b4 = float(square['X4']['kb']) - k4 * float(square['X4']['ka'])

            y1 = k1 * big_point_dict['ka'] + b1
            y3 = k3 * big_point_dict['ka'] + b3
            x2 = (big_point_dict['kb'] - b2) / k2
            x4 = (big_point_dict['kb'] - b4) / k4

            if y1 <= big_point_dict['kb'] <= y3 and x2 >= big_point_dict['ka'] >= x4:
                d1 = math.fabs(((-k1) * big_point_dict['ka'] + big_point_dict['kb'] - b1) / math.sqrt(k1 * k1 + 1))
                d2 = math.fabs(((-k2) * big_point_dict['ka'] + big_point_dict['kb'] - b2) / math.sqrt(k2 * k2 + 1))
                d3 = math.fabs(((-k3) * big_point_dict['ka'] + big_point_dict['kb'] - b3) / math.sqrt(k3 * k3 + 1))
                d4 = math.fabs(((-k4) * big_point_dict['ka'] + big_point_dict['kb'] - b4) / math.sqrt(k4 * k4 + 1))

                big_point_dict['b'] = square['X1']['b'] - d1 / (d1 + d3) * 5
                big_point_dict['a'] = square['X1']['a'] + d4 / (d2 + d4) * 5
                return big_point_dict

    except (KeyError, ValueError, ZeroDivisionError) as e:
        print(f"Exception occurred: {e}")

    return None


"""
def big_create_square(big_calibration_dict, n):
    big_square_list = [{'X1': {}, 'X2': {}, 'X3': {}, 'X4': {}} for _ in range(36)]

    for j in range(12):
        jj = 60 * int(n) - 30 - 5 * j
        if jj < 0:
            jj += 360

        for i in range(3):
            ii = 5 * i + 30
            big_square_list[3 * j + i]['X1']['a'] = ii
            big_square_list[3 * j + i]['X1']['b'] = jj
            big_square_list[3 * j + i]['X2']['a'] = ii + 5
            big_square_list[3 * j + i]['X2']['b'] = jj
            big_square_list[3 * j + i]['X3']['a'] = ii + 5
            big_square_list[3 * j + i]['X4']['a'] = ii
            if jj == 0:
                big_square_list[3 * j + i]['X3']['b'] = 355
                big_square_list[3 * j + i]['X4']['b'] = 355
            else:
                big_square_list[3 * j + i]['X3']['b'] = jj - 5
                big_square_list[3 * j + i]['X4']['b'] = jj - 5

            k = 0
            for calibration in big_calibration_dict[n]:
                if jj == int(calibration['b']) and ii == int(calibration['a']):
                    big_square_list[3 * j + i]['X1']['ka'] = calibration['ka']
                    big_square_list[3 * j + i]['X1']['kb'] = calibration['kb']
                    k += 1
                    continue
                if jj == int(calibration['b']) and ii + 5 == int(calibration['a']):
                    big_square_list[3 * j + i]['X2']['ka'] = calibration['ka']
                    big_square_list[3 * j + i]['X2']['kb'] = calibration['kb']
                    k += 1
                    continue
                if jj != 0 and jj - 5 == int(calibration['b']) and ii + 5 == int(calibration['a']):
                    big_square_list[3 * j + i]['X3']['ka'] = calibration['ka']
                    big_square_list[3 * j + i]['X3']['kb'] = calibration['kb']
                    k += 1
                    continue
                elif jj == 0 and 355 == int(calibration['b']) and ii + 5 == int(calibration['a']):
                    big_square_list[3 * j + i]['X3']['ka'] = calibration['ka']
                    big_square_list[3 * j + i]['X3']['kb'] = calibration['kb']
                    k += 1
                    continue
                if jj != 0 and jj - 5 == int(calibration['b']) and ii == int(calibration['a']):
                    big_square_list[3 * j + i]['X4']['ka'] = calibration['ka']
                    big_square_list[3 * j + i]['X4']['kb'] = calibration['kb']
                    k += 1
                    continue
                elif jj == 0 and 355 == int(calibration['b']) and ii == int(calibration['a']):
                    big_square_list[3 * j + i]['X4']['ka'] = calibration['ka']
                    big_square_list[3 * j + i]['X4']['kb'] = calibration['kb']
                    k += 1
                    continue
                if k == 4:
                    break

    return big_square_list
"""
def big_create_square(big_calibration_dict, n):
    try:
        # 初始化大方块列表
        big_square_list = [{'X1': {}, 'X2': {}, 'X3': {}, 'X4': {}} for _ in range(36)]

        # 遍历大方块列表并填充数据
        for j in range(12):
            jj = 60 * int(n) - 30 - 5 * j
            if jj < 0:
                jj += 360

            for i in range(3):
                ii = 5 * i + 30
                big_square_list[3 * j + i]['X1']['a'] = ii
                big_square_list[3 * j + i]['X1']['b'] = jj
                big_square_list[3 * j + i]['X2']['a'] = ii + 5
                big_square_list[3 * j + i]['X2']['b'] = jj
                big_square_list[3 * j + i]['X3']['a'] = ii + 5
                big_square_list[3 * j + i]['X4']['a'] = ii
                if jj == 0:
                    big_square_list[3 * j + i]['X3']['b'] = 355
                    big_square_list[3 * j + i]['X4']['b'] = 355
                else:
                    big_square_list[3 * j + i]['X3']['b'] = jj - 5
                    big_square_list[3 * j + i]['X4']['b'] = jj - 5

                k = 0
                for calibration in big_calibration_dict.get(n, []):
                    if jj == int(calibration['b']) and ii == int(calibration['a']):
                        big_square_list[3 * j + i]['X1']['ka'] = calibration['ka']
                        big_square_list[3 * j + i]['X1']['kb'] = calibration['kb']
                        k += 1
                        continue
                    if jj == int(calibration['b']) and ii + 5 == int(calibration['a']):
                        big_square_list[3 * j + i]['X2']['ka'] = calibration['ka']
                        big_square_list[3 * j + i]['X2']['kb'] = calibration['kb']
                        k += 1
                        continue
                    if jj != 0 and jj - 5 == int(calibration['b']) and ii + 5 == int(calibration['a']):
                        big_square_list[3 * j + i]['X3']['ka'] = calibration['ka']
                        big_square_list[3 * j + i]['X3']['kb'] = calibration['kb']
                        k += 1
                        continue
                    elif jj == 0 and 355 == int(calibration['b']) and ii + 5 == int(calibration['a']):
                        big_square_list[3 * j + i]['X3']['ka'] = calibration['ka']
                        big_square_list[3 * j + i]['X3']['kb'] = calibration['kb']
                        k += 1
                        continue
                    if jj != 0 and jj - 5 == int(calibration['b']) and ii == int(calibration['a']):
                        big_square_list[3 * j + i]['X4']['ka'] = calibration['ka']
                        big_square_list[3 * j + i]['X4']['kb'] = calibration['kb']
                        k += 1
                        continue
                    elif jj == 0 and 355 == int(calibration['b']) and ii == int(calibration['a']):
                        big_square_list[3 * j + i]['X4']['ka'] = calibration['ka']
                        big_square_list[3 * j + i]['X4']['kb'] = calibration['kb']
                        k += 1
                        continue
                    if k == 4:
                        break

        return big_square_list

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except TypeError as e:
        print(f"TypeError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None

"""
def big_create_line(big_calibration_dict, n):
    ab_dict = {
        '1': (330, 30),
        '2': (30, 90),
        '3': (90, 150),
        '4': (150, 210),
        '5': (210, 270),
        '6': (270, 330)
    }
    if n not in ab_dict:
        return None

    big_line_list = [[{} for _ in range(4)], [{} for _ in range(13)], [{} for _ in range(4)], [{} for _ in range(13)]]
    for calibration in big_calibration_dict[n]:
        for j in range(4):
            jj = 30 + 5 * j

            if ab_dict[n][1] == int(calibration['b']) and jj == int(calibration['a']):
                big_line_list[0][j]['a'] = calibration['a']
                big_line_list[0][j]['b'] = calibration['b']
                big_line_list[0][j]['ka'] = calibration['ka']
                big_line_list[0][j]['kb'] = calibration['kb']

            if ab_dict[n][0] == int(calibration['b']) and jj == int(calibration['a']):
                big_line_list[2][j]['a'] = calibration['a']
                big_line_list[2][j]['b'] = calibration['b']
                big_line_list[2][j]['ka'] = calibration['ka']
                big_line_list[2][j]['kb'] = calibration['kb']

        for i in range(13):
            ii = 5 * i + 60 * int(n) - 90
            if ii < 0:
                ii += 360

            if ii == int(calibration['b']) and 45 == int(calibration['a']):
                big_line_list[1][i]['a'] = calibration['a']
                big_line_list[1][i]['b'] = calibration['b']
                big_line_list[1][i]['ka'] = calibration['ka']
                big_line_list[1][i]['kb'] = calibration['kb']

            if ii == int(calibration['b']) and 30 == int(calibration['a']):
                big_line_list[3][i]['a'] = calibration['a']
                big_line_list[3][i]['b'] = calibration['b']
                big_line_list[3][i]['ka'] = calibration['ka']
                big_line_list[3][i]['kb'] = calibration['kb']

    big_line_list[1].reverse()
    big_line_list[2].reverse()

    return big_line_list
"""
def big_create_line(big_calibration_dict, n):
    try:
        # 定义字典
        ab_dict = {
            '1': (330, 30),
            '2': (30, 90),
            '3': (90, 150),
            '4': (150, 210),
            '5': (210, 270),
            '6': (270, 330)
        }
        
        # 检查n是否在字典中
        if n not in ab_dict:
            raise ValueError(f"无效的n值: {n}")

        # 初始化大线列表
        big_line_list = [[{} for _ in range(4)], [{} for _ in range(13)], [{} for _ in range(4)], [{} for _ in range(13)]]

        # 遍历校准字典
        for calibration in big_calibration_dict.get(n, []):
            for j in range(4):
                jj = 30 + 5 * j

                if ab_dict[n][1] == int(calibration['b']) and jj == int(calibration['a']):
                    big_line_list[0][j]['a'] = calibration['a']
                    big_line_list[0][j]['b'] = calibration['b']
                    big_line_list[0][j]['ka'] = calibration['ka']
                    big_line_list[0][j]['kb'] = calibration['kb']

                if ab_dict[n][0] == int(calibration['b']) and jj == int(calibration['a']):
                    big_line_list[2][j]['a'] = calibration['a']
                    big_line_list[2][j]['b'] = calibration['b']
                    big_line_list[2][j]['ka'] = calibration['ka']
                    big_line_list[2][j]['kb'] = calibration['kb']

            for i in range(13):
                ii = 5 * i + 60 * int(n) - 90
                if ii < 0:
                    ii += 360

                if ii == int(calibration['b']) and 45 == int(calibration['a']):
                    big_line_list[1][i]['a'] = calibration['a']
                    big_line_list[1][i]['b'] = calibration['b']
                    big_line_list[1][i]['ka'] = calibration['ka']
                    big_line_list[1][i]['kb'] = calibration['kb']

                if ii == int(calibration['b']) and 30 == int(calibration['a']):
                    big_line_list[3][i]['a'] = calibration['a']
                    big_line_list[3][i]['b'] = calibration['b']
                    big_line_list[3][i]['ka'] = calibration['ka']
                    big_line_list[3][i]['kb'] = calibration['kb']

        # 反转列表
        big_line_list[1].reverse()
        big_line_list[2].reverse()

        return big_line_list

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except TypeError as e:
        print(f"TypeError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None

"""
def big_max_pressure(hole_dict):
    p1 = hole_dict['p1']
    p2 = hole_dict['p2']
    p3 = hole_dict['p3']
    p4 = hole_dict['p4']
    p5 = hole_dict['p5']
    p6 = hole_dict['p6']
    sorted_values = sorted([p1, p2, p3, p4, p5, p6])
    max_values = sorted_values[4:6]
    max_keys = {
        'first': list(hole_dict.keys())[list(hole_dict.values()).index(max_values[1])][-1],
        'second': list(hole_dict.keys())[list(hole_dict.values()).index(max_values[0])][-1]
    }
    return max_keys
"""
def big_max_pressure(hole_dict):
    try:
        # 检查字典中是否包含所需的键
        required_keys = ['p1', 'p2', 'p3', 'p4', 'p5', 'p6']
        for key in required_keys:
            if key not in hole_dict:
                raise KeyError(f"缺少hole_dict中的关键键: {key}")

        # 提取数据
        p1 = hole_dict['p1']
        p2 = hole_dict['p2']
        p3 = hole_dict['p3']
        p4 = hole_dict['p4']
        p5 = hole_dict['p5']
        p6 = hole_dict['p6']

        # 确保值是数字类型
        pressures = [p1, p2, p3, p4, p5, p6]
        if not all(isinstance(p, (int, float)) for p in pressures):
            raise TypeError("hole_dict中的所有值必须是数字类型")

        # 排序并找出最大值
        sorted_values = sorted(pressures)
        max_values = sorted_values[4:6]

        # 获取最大值对应的键
        max_keys = {
            'first': list(hole_dict.keys())[list(hole_dict.values()).index(max_values[1])][-1],
            'second': list(hole_dict.keys())[list(hole_dict.values()).index(max_values[0])][-1]
        }

        return max_keys

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except TypeError as e:
        print(f"TypeError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None

"""
def big_cal_kakb(hole_dict, n):
    big_point_dict = {}
    p7 = hole_dict['p7']
    pcenter = hole_dict['p' + n]
    if int(n) == 1:
        pleft = hole_dict['p6']
    else:
        pleft = hole_dict['p' + str(int(n) - 1)]
    if int(n) == 6:
        pright = hole_dict['p1']
    else:
        pright = hole_dict['p' + str(int(n) + 1)]

    ka = (pcenter - p7) / (pcenter - (pleft + pright) / 2)
    kb = (pleft - pright) / (pcenter - (pleft + pright) / 2)

    big_point_dict['ka'] = ka
    big_point_dict['kb'] = kb

    return big_point_dict

"""
def big_cal_kakb(hole_dict, n):
    try:
        # 检查字典中是否包含所需的键
        required_keys = ['p7']
        required_keys.append(f'p{n}')
        for key in required_keys:
            if key not in hole_dict:
                raise KeyError(f"缺少hole_dict中的关键键: {key}")

        p7 = hole_dict['p7']
        pcenter = hole_dict[f'p{n}']

        if int(n) == 1:
            if 'p6' not in hole_dict:
                raise KeyError("缺少hole_dict中的关键键: p6")
            pleft = hole_dict['p6']
        else:
            if f'p{int(n) - 1}' not in hole_dict:
                raise KeyError(f"缺少hole_dict中的关键键: p{int(n) - 1}")
            pleft = hole_dict[f'p{int(n) - 1}']

        if int(n) == 6:
            if 'p1' not in hole_dict:
                raise KeyError("缺少hole_dict中的关键键: p1")
            pright = hole_dict['p1']
        else:
            if f'p{int(n) + 1}' not in hole_dict:
                raise KeyError(f"缺少hole_dict中的关键键: p{int(n) + 1}")
            pright = hole_dict[f'p{int(n) + 1}']

        # 检查是否存在除数为零的情况
        denominator = pcenter - (pleft + pright) / 2
        if denominator == 0:
            raise ZeroDivisionError("pcenter 与 (pleft + pright) / 2 相等，导致除数为零")

        # 计算ka和kb
        ka = (pcenter - p7) / denominator
        kb = (pleft - pright) / denominator

        # 返回结果
        big_point_dict = {'ka': ka, 'kb': kb}
        return big_point_dict

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ZeroDivisionError as e:
        print(f"ZeroDivisionError: {e}")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except TypeError as e:
        print(f"TypeError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None


"""
def little_cal_kakb(hole_dict):
    little_point_dict = {}
    p1 = hole_dict['p1']
    p2 = hole_dict['p2']
    p3 = hole_dict['p3']
    p4 = hole_dict['p4']
    p5 = hole_dict['p5']
    p6 = hole_dict['p6']
    p7 = hole_dict['p7']

    p_average = (p1 + p2 + p3 + p4 + p5 + p6) / 6.0
    cpa = (p4 - p1) / (p7 - p_average)
    cpb = (p5 - p2) / (p7 - p_average)
    cpc = (p6 - p3) / (p7 - p_average)

    ka = (cpb + cpc) / math.sqrt(3)
    kb = -(2 * cpa + cpb - cpc) / 3

    little_point_dict['ka'] = ka
    little_point_dict['kb'] = kb

    return little_point_dict
"""

def little_cal_kakb(hole_dict):
    try:
        # 检查字典中是否包含所需的键
        required_keys = ['p1', 'p2', 'p3', 'p4', 'p5', 'p6', 'p7']
        for key in required_keys:
            if key not in hole_dict:
                raise KeyError(f"缺少hole_dict中的关键键: {key}")

        # 提取数据
        p1 = hole_dict['p1']
        p2 = hole_dict['p2']
        p3 = hole_dict['p3']
        p4 = hole_dict['p4']
        p5 = hole_dict['p5']
        p6 = hole_dict['p6']
        p7 = hole_dict['p7']

        # 计算平均值
        p_average = (p1 + p2 + p3 + p4 + p5 + p6) / 6.0

        # 检查是否存在除数为零的情况
        if p7 == p_average:
            raise ZeroDivisionError("p7 与 p_average 相等，导致除数为零")

        # 计算cpa, cpb, cpc
        cpa = (p4 - p1) / (p7 - p_average)
        cpb = (p5 - p2) / (p7 - p_average)
        cpc = (p6 - p3) / (p7 - p_average)

        # 计算ka和kb
        ka = (cpb + cpc) / math.sqrt(3)
        kb = -(2 * cpa + cpb - cpc) / 3

        # 返回结果
        little_point_dict = {'ka': ka, 'kb': kb}
        return little_point_dict

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ZeroDivisionError as e:
        print(f"ZeroDivisionError: {e}")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except TypeError as e:
        print(f"TypeError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None


"""
def little_cal_ab(little_point_dict, little_square_list):
    for square in little_square_list:
        k1 = (float(square['X2']['kb']) - float(square['X1']['kb'])) / (
                float(square['X2']['ka']) - float(square['X1']['ka']))
        b1 = float(square['X1']['kb']) - k1 * float(square['X1']['ka'])
        k2 = (float(square['X3']['kb']) - float(square['X2']['kb'])) / (
                float(square['X3']['ka']) - float(square['X2']['ka']))
        b2 = float(square['X2']['kb']) - k2 * float(square['X2']['ka'])
        k3 = (float(square['X4']['kb']) - float(square['X3']['kb'])) / (
                float(square['X4']['ka']) - float(square['X3']['ka']))
        b3 = float(square['X3']['kb']) - k3 * float(square['X3']['ka'])
        k4 = (float(square['X1']['kb']) - float(square['X4']['kb'])) / (
                float(square['X1']['ka']) - float(square['X4']['ka']))
        b4 = float(square['X4']['kb']) - k4 * float(square['X4']['ka'])

        y1 = k1 * little_point_dict['ka'] + b1
        y3 = k3 * little_point_dict['ka'] + b3
        x2 = (little_point_dict['kb'] - b2) / k2
        x4 = (little_point_dict['kb'] - b4) / k4
        if y1 <= little_point_dict['kb'] <= y3 and x2 >= little_point_dict['ka'] >= x4:
            d1 = math.fabs(((-k1) * little_point_dict['ka'] + little_point_dict['kb'] - b1) / math.sqrt(k1 * k1 + 1))
            d2 = math.fabs(((-k2) * little_point_dict['ka'] + little_point_dict['kb'] - b2) / math.sqrt(k2 * k2 + 1))
            d3 = math.fabs(((-k3) * little_point_dict['ka'] + little_point_dict['kb'] - b3) / math.sqrt(k3 * k3 + 1))
            d4 = math.fabs(((-k4) * little_point_dict['ka'] + little_point_dict['kb'] - b4) / math.sqrt(k4 * k4 + 1))

            little_point_dict['b'] = square['X1']['b'] + d1 / (d1 + d3) * 5
            little_point_dict['a'] = square['X1']['a'] + d4 / (d2 + d4) * 5
            return little_point_dict
"""

def little_cal_ab(little_point_dict, little_square_list):
    try:
        for square in little_square_list:
            # 检查字典中是否包含所需的键
            required_keys = ['X1', 'X2', 'X3', 'X4']
            for key in required_keys:
                if key not in square:
                    raise KeyError(f"缺少square中的关键键: {key}")
                if 'ka' not in square[key] or 'kb' not in square[key]:
                    raise KeyError(f"缺少square['{key}']中的关键键: 'ka' 或 'kb'")

            # 计算k1和b1
            k1 = (float(square['X2']['kb']) - float(square['X1']['kb'])) / (
                float(square['X2']['ka']) - float(square['X1']['ka']))
            b1 = float(square['X1']['kb']) - k1 * float(square['X1']['ka'])

            # 计算k2和b2
            k2 = (float(square['X3']['kb']) - float(square['X2']['kb'])) / (
                float(square['X3']['ka']) - float(square['X2']['ka']))
            b2 = float(square['X2']['kb']) - k2 * float(square['X2']['ka'])

            # 计算k3和b3
            k3 = (float(square['X4']['kb']) - float(square['X3']['kb'])) / (
                float(square['X4']['ka']) - float(square['X3']['ka']))
            b3 = float(square['X3']['kb']) - k3 * float(square['X3']['ka'])

            # 计算k4和b4
            k4 = (float(square['X1']['kb']) - float(square['X4']['kb'])) / (
                float(square['X1']['ka']) - float(square['X4']['ka']))
            b4 = float(square['X4']['kb']) - k4 * float(square['X4']['ka'])

            # 计算y1和y3
            y1 = k1 * little_point_dict['ka'] + b1
            y3 = k3 * little_point_dict['ka'] + b3

            # 计算x2和x4
            x2 = (little_point_dict['kb'] - b2) / k2
            x4 = (little_point_dict['kb'] - b4) / k4

            # 检查点是否在多边形内
            if y1 <= little_point_dict['kb'] <= y3 and x2 >= little_point_dict['ka'] >= x4:
                # 计算d1, d2, d3, d4
                d1 = math.fabs(((-k1) * little_point_dict['ka'] + little_point_dict['kb'] - b1) / math.sqrt(k1 * k1 + 1))
                d2 = math.fabs(((-k2) * little_point_dict['ka'] + little_point_dict['kb'] - b2) / math.sqrt(k2 * k2 + 1))
                d3 = math.fabs(((-k3) * little_point_dict['ka'] + little_point_dict['kb'] - b3) / math.sqrt(k3 * k3 + 1))
                d4 = math.fabs(((-k4) * little_point_dict['ka'] + little_point_dict['kb'] - b4) / math.sqrt(k4 * k4 + 1))

                # 更新little_point_dict
                if 'b' not in square['X1'] or 'a' not in square['X1']:
                    raise KeyError("square['X1']缺少关键键: 'b' 或 'a'")
                little_point_dict['b'] = square['X1']['b'] + d1 / (d1 + d3) * 5
                little_point_dict['a'] = square['X1']['a'] + d4 / (d2 + d4) * 5
                return little_point_dict

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ZeroDivisionError as e:
        print("ZeroDivisionError: 除数不能为零")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except TypeError as e:
        print(f"TypeError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None


"""
def point_in_polygon(little_point_dict, little_line_list):
    if 'ka' and 'kb' in little_point_dict:
        point_tuple = (little_point_dict['ka'], little_point_dict['kb'])
    else:
        point_tuple = None
    edge_list = little_line_list[0] + little_line_list[1] + little_line_list[2] + little_line_list[3]
    edge_tuple_list = [(d.get('ka'), d.get('kb')) for d in edge_list]
    edge_tuple = []
    for edge in edge_tuple_list:
        if edge not in edge_tuple:
            edge_tuple.append(edge)

    x, y = point_tuple
    inside = False
    on_boundary = False
    n = len(edge_tuple)
    p1x, p1y = edge_tuple[0]
    if p1x is not None and p1y is not None:
        p1x = float(p1x)
        p1y = float(p1y)
    for i in range(n + 1):
        p2x, p2y = edge_tuple[i % n]
        if p2x is not None and p2y is not None:
            p2x = float(p2x)
            p2y = float(p2y)
        if y == p1y and y == p2y:
            if min(p1x, p2x) <= x <= max(p1x, p2x):
                on_boundary = True
                break
        if p1y and p2y and min(p1y, p2y) < y <= max(p1y, p2y):
            if x <= max(p1x, p2x):
                if p1y != p2y:
                    x_inters = (y - p1y) * (p2x - p1x) / (p2y - p1y) + p1x
                    if p1x == p2x or x <= x_inters:
                        inside = not inside
        p1x, p1y = p2x, p2y
    if on_boundary:
        return 0
    elif inside:
        return 1
    else:
        return -1
"""
def point_in_polygon(little_point_dict, little_line_list):
    try:
        # 检查字典中是否包含所需的键
        if 'ka' in little_point_dict and 'kb' in little_point_dict:
            point_tuple = (little_point_dict['ka'], little_point_dict['kb'])
        else:
            raise KeyError("little_point_dict缺少'ka'或'kb'键")

        edge_list = little_line_list[0] + little_line_list[1] + little_line_list[2] + little_line_list[3]
        edge_tuple_list = [(d.get('ka'), d.get('kb')) for d in edge_list]

        edge_tuple = []
        for edge in edge_tuple_list:
            if edge not in edge_tuple:
                edge_tuple.append(edge)

        x, y = point_tuple
        inside = False
        on_boundary = False
        n = len(edge_tuple)
        if n == 0:
            raise ValueError("边列表不能为空")
            
        p1x, p1y = edge_tuple[0]
        if p1x is None or p1y is None:
            raise ValueError("edge_tuple包含None值")
        p1x = float(p1x)
        p1y = float(p1y)

        for i in range(n + 1):
            p2x, p2y = edge_tuple[i % n]
            if p2x is None or p2y is None:
                raise ValueError("edge_tuple包含None值")
            p2x = float(p2x)
            p2y = float(p2y)

            if y == p1y and y == p2y:
                if min(p1x, p2x) <= x <= max(p1x, p2x):
                    on_boundary = True
                    break

            if min(p1y, p2y) < y <= max(p1y, p2y):
                if x <= max(p1x, p2x):
                    if p1y != p2y:
                        x_inters = (y - p1y) * (p2x - p1x) / (p2y - p1y) + p1x
                        if p1x == p2x or x <= x_inters:
                            inside = not inside

            p1x, p1y = p2x, p2y

        if on_boundary:
            return 0
        elif inside:
            return 1
        else:
            return -1

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except TypeError as e:
        print(f"TypeError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None

"""
def little_create_square(little_calibration_list):
    little_square_list = [{'X1': {}, 'X2': {}, 'X3': {}, 'X4': {}} for _ in range(144)]

    for j in range(12):
        jj = 5 * j - 30

        for i in range(12):
            ii = 5 * i - 30
            little_square_list[12 * j + i]['X1']['a'] = ii
            little_square_list[12 * j + i]['X1']['b'] = jj
            little_square_list[12 * j + i]['X2']['a'] = ii + 5
            little_square_list[12 * j + i]['X2']['b'] = jj
            little_square_list[12 * j + i]['X3']['a'] = ii + 5
            little_square_list[12 * j + i]['X3']['b'] = jj + 5
            little_square_list[12 * j + i]['X4']['a'] = ii
            little_square_list[12 * j + i]['X4']['b'] = jj + 5

            k = 0
            for calibration in little_calibration_list:
                if jj == int(calibration['b']) and ii == int(calibration['a']):
                    little_square_list[12 * j + i]['X1']['ka'] = calibration['ka']
                    little_square_list[12 * j + i]['X1']['kb'] = calibration['kb']
                    k += 1
                    continue
                if jj == int(calibration['b']) and ii + 5 == int(calibration['a']):
                    little_square_list[12 * j + i]['X2']['ka'] = calibration['ka']
                    little_square_list[12 * j + i]['X2']['kb'] = calibration['kb']
                    k += 1
                    continue
                if jj + 5 == int(calibration['b']) and ii + 5 == int(calibration['a']):
                    little_square_list[12 * j + i]['X3']['ka'] = calibration['ka']
                    little_square_list[12 * j + i]['X3']['kb'] = calibration['kb']
                    k += 1
                    continue
                if jj + 5 == int(calibration['b']) and ii == int(calibration['a']):
                    little_square_list[12 * j + i]['X4']['ka'] = calibration['ka']
                    little_square_list[12 * j + i]['X4']['kb'] = calibration['kb']
                    k += 1
                    continue
                if k == 4:
                    break

    return little_square_list
"""
def little_create_square(little_calibration_list):
    try:
        little_square_list = [{'X1': {}, 'X2': {}, 'X3': {}, 'X4': {}} for _ in range(144)]

        for j in range(12):
            jj = 5 * j - 30

            for i in range(12):
                ii = 5 * i - 30
                little_square_list[12 * j + i]['X1']['a'] = ii
                little_square_list[12 * j + i]['X1']['b'] = jj
                little_square_list[12 * j + i]['X2']['a'] = ii + 5
                little_square_list[12 * j + i]['X2']['b'] = jj
                little_square_list[12 * j + i]['X3']['a'] = ii + 5
                little_square_list[12 * j + i]['X3']['b'] = jj + 5
                little_square_list[12 * j + i]['X4']['a'] = ii
                little_square_list[12 * j + i]['X4']['b'] = jj + 5

                k = 0
                for calibration in little_calibration_list:
                    try:
                        if jj == int(calibration['b']) and ii == int(calibration['a']):
                            little_square_list[12 * j + i]['X1']['ka'] = calibration['ka']
                            little_square_list[12 * j + i]['X1']['kb'] = calibration['kb']
                            k += 1
                            continue
                        if jj == int(calibration['b']) and ii + 5 == int(calibration['a']):
                            little_square_list[12 * j + i]['X2']['ka'] = calibration['ka']
                            little_square_list[12 * j + i]['X2']['kb'] = calibration['kb']
                            k += 1
                            continue
                        if jj + 5 == int(calibration['b']) and ii + 5 == int(calibration['a']):
                            little_square_list[12 * j + i]['X3']['ka'] = calibration['ka']
                            little_square_list[12 * j + i]['X3']['kb'] = calibration['kb']
                            k += 1
                            continue
                        if jj + 5 == int(calibration['b']) and ii == int(calibration['a']):
                            little_square_list[12 * j + i]['X4']['ka'] = calibration['ka']
                            little_square_list[12 * j + i]['X4']['kb'] = calibration['kb']
                            k += 1
                            continue
                        if k == 4:
                            break
                    except KeyError as e:
                        print(f"KeyError: {e} in calibration {calibration}")
                    except ValueError as e:
                        print(f"ValueError: {e} in calibration {calibration}")
                    except Exception as e:
                        print(f"Unexpected error: {e} in calibration {calibration}")

        return little_square_list

    except Exception as e:
        print(f"An error occurred: {e}")
        return []

"""
def little_create_line(little_calibration_list):
    little_line_list = [[{} for _ in range(13)] for _ in range(4)]

    for i in range(13):
        ii = 5 * i - 30
        for calibration in little_calibration_list:

            if -30 == int(calibration['b']) and ii == int(calibration['a']):
                little_line_list[0][i]['a'] = calibration['a']
                little_line_list[0][i]['b'] = calibration['b']
                little_line_list[0][i]['ka'] = calibration['ka']
                little_line_list[0][i]['kb'] = calibration['kb']

            if ii == int(calibration['b']) and 30 == int(calibration['a']):
                little_line_list[1][i]['a'] = calibration['a']
                little_line_list[1][i]['b'] = calibration['b']
                little_line_list[1][i]['ka'] = calibration['ka']
                little_line_list[1][i]['kb'] = calibration['kb']

            if 30 == int(calibration['b']) and ii == int(calibration['a']):
                little_line_list[2][i]['a'] = calibration['a']
                little_line_list[2][i]['b'] = calibration['b']
                little_line_list[2][i]['ka'] = calibration['ka']
                little_line_list[2][i]['kb'] = calibration['kb']

            if ii == int(calibration['b']) and -30 == int(calibration['a']):
                little_line_list[3][i]['a'] = calibration['a']
                little_line_list[3][i]['b'] = calibration['b']
                little_line_list[3][i]['ka'] = calibration['ka']
                little_line_list[3][i]['kb'] = calibration['kb']

    little_line_list[2].reverse()
    little_line_list[3].reverse()

    return little_line_list
"""
def little_create_line(little_calibration_list):
    try:
        little_line_list = [[{} for _ in range(13)] for _ in range(4)]

        for i in range(13):
            ii = 5 * i - 30
            for calibration in little_calibration_list:
                try:
                    if -30 == int(calibration['b']) and ii == int(calibration['a']):
                        little_line_list[0][i]['a'] = calibration['a']
                        little_line_list[0][i]['b'] = calibration['b']
                        little_line_list[0][i]['ka'] = calibration['ka']
                        little_line_list[0][i]['kb'] = calibration['kb']

                    if ii == int(calibration['b']) and 30 == int(calibration['a']):
                        little_line_list[1][i]['a'] = calibration['a']
                        little_line_list[1][i]['b'] = calibration['b']
                        little_line_list[1][i]['ka'] = calibration['ka']
                        little_line_list[1][i]['kb'] = calibration['kb']

                    if 30 == int(calibration['b']) and ii == int(calibration['a']):
                        little_line_list[2][i]['a'] = calibration['a']
                        little_line_list[2][i]['b'] = calibration['b']
                        little_line_list[2][i]['ka'] = calibration['ka']
                        little_line_list[2][i]['kb'] = calibration['kb']

                    if ii == int(calibration['b']) and -30 == int(calibration['a']):
                        little_line_list[3][i]['a'] = calibration['a']
                        little_line_list[3][i]['b'] = calibration['b']
                        little_line_list[3][i]['ka'] = calibration['ka']
                        little_line_list[3][i]['kb'] = calibration['kb']
                except KeyError as e:
                    print(f"KeyError: {e} in calibration {calibration}")
                except ValueError as e:
                    print(f"ValueError: {e} in calibration {calibration}")
                except Exception as e:
                    print(f"Unexpected error: {e} in calibration {calibration}")

        little_line_list[2].reverse()
        little_line_list[3].reverse()

        return little_line_list

    except Exception as e:
        print(f"An error occurred: {e}")
        return []



"""
def big_cal_cptcps(big_point_dict, big_square_dict):
    cptcps_list = ['cpt', 'cps']
    for cptcps in cptcps_list:
        k1 = (float(big_square_dict['X2'][cptcps]) - float(big_square_dict['X1'][cptcps])) / (
                float(big_square_dict['X2']['b']) - float(big_square_dict['X1']['b']))
        b1 = big_square_dict['X1'][cptcps] - k1 * big_square_dict['X1']['b']
        k3 = (float(big_square_dict['X4'][cptcps]) - float(big_square_dict['X3'][cptcps])) / (
                float(big_square_dict['X4']['b']) - float(big_square_dict['X3']['b']))
        b3 = float(big_square_dict['X3'][cptcps]) - k3 * float(big_square_dict['X3']['b'])

        cp1 = k1 * big_point_dict['b'] + b1
        cp2 = k3 * big_point_dict['b'] + b3
        cp = (cp2 - cp1) * (big_point_dict['a'] - big_square_dict['X1']['a']) / 5 + cp1
        big_point_dict[cptcps] = cp

    return big_point_dict
"""
def big_cal_cptcps(big_point_dict, big_square_dict):
    try:
        cptcps_list = ['cpt', 'cps']
        
        for cptcps in cptcps_list:
            # 检查字典中是否包含所需的键
            required_keys_square = ['X1', 'X2', 'X3', 'X4']
            for key in required_keys_square:
                if key not in big_square_dict:
                    raise KeyError(f"缺少big_square_dict中的关键键: {key}")
                if cptcps not in big_square_dict[key]:
                    raise KeyError(f"缺少big_square_dict['{key}']中的关键键: {cptcps}")
                if 'b' not in big_square_dict[key]:
                    raise KeyError(f"缺少big_square_dict['{key}']中的关键键: 'b'")
            if 'a' not in big_square_dict['X1']:
                raise KeyError("缺少big_square_dict['X1']中的关键键: 'a'")
            
            if 'a' not in big_point_dict or 'b' not in big_point_dict:
                raise KeyError("缺少big_point_dict中的关键键: 'a' 或 'b'")
            
            # 计算k1和b1
            k1 = (float(big_square_dict['X2'][cptcps]) - float(big_square_dict['X1'][cptcps])) / (
                float(big_square_dict['X2']['b']) - float(big_square_dict['X1']['b']))
            b1 = float(big_square_dict['X1'][cptcps]) - k1 * float(big_square_dict['X1']['b'])

            # 计算k3和b3
            k3 = (float(big_square_dict['X4'][cptcps]) - float(big_square_dict['X3'][cptcps])) / (
                float(big_square_dict['X4']['b']) - float(big_square_dict['X3']['b']))
            b3 = float(big_square_dict['X3'][cptcps]) - k3 * float(big_square_dict['X3']['b'])

            # 计算cp1和cp2
            cp1 = k1 * float(big_point_dict['b']) + b1
            cp2 = k3 * float(big_point_dict['b']) + b3

            # 计算cp
            cp = (cp2 - cp1) * (float(big_point_dict['a']) - float(big_square_dict['X1']['a'])) / 5 + cp1
            big_point_dict[cptcps] = cp

    except KeyError as e:
        print(f"KeyError: {e}")
        return None
    except ZeroDivisionError as e:
        print("ZeroDivisionError: 除数不能为零")
        return None
    except ValueError as e:
        print(f"ValueError: {e}")
        return None
    except Exception as e:
        print(f"Unexpected error: {e}")
        return None

    return big_point_dict
